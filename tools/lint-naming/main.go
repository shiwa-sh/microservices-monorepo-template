// Command lint-naming enforces the resource-name grammar (ADR-0003).
//
// Every named resource derives from `{project}-{env}-{role}[-{n}]`, the slug
// matches `^[a-z][a-z0-9]*(-[a-z0-9]+)*$` and is at most 63 characters, the project
// slug is 6–11 characters, `env` is spelled out, and `role` comes from a closed
// table. None of that is checkable by the cluster — a provider accepts any name it
// can encode, and a wrong one is discovered when someone reads a bill — so it is
// CI's to enforce or nobody's.
//
// # What it reads
//
//	infra/ansible/inventory/<env>/hosts.yml   the host names, which become the
//	                                          machine names, the SSH aliases, and
//	                                          the Kubernetes node names
//	.sops.yaml                                the env token in every recipient
//	                                          anchor and path rule
//	infra/gitops/platform/<env>/              the per-environment overlay directories
//
// These are the surfaces where an environment is NAMED rather than referenced. A
// misspelling in any of them ("stg", "prd") is the specific failure ADR-0003's "no
// abbreviations" rule exists to prevent: it forces a mapping table between surfaces,
// and the mapping is what rots.
//
// # The role table is parsed, not copied
//
// The closed vocabulary lives in ADR-0003 as a markdown table, and this reads it
// from there. A copy here would be a second source of truth for a list whose whole
// purpose is to be the only one — and the ADR's own rule is that a new resource
// class adds a row in the same PR, which only means something if the row is what
// the gate consults.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const adrFile = "docs/adr/0003-naming-and-identifiers.md"

const inventoryDir = "infra/ansible/inventory"

const sopsFile = ".sops.yaml"

const overlayDir = "infra/gitops/platform"

// provisionedEnvs is ADR-0200's environment set, verbatim. `env` is the
// environment's own name and nothing else.
var provisionedEnvs = map[string]bool{"dev": true, "staging": true, "prod": true}

// localTier is the laptop tier (ADR-0600). It is not a provisioned environment and
// has no provider resources, so it never appears in an inventory — but it does own
// a GitOps overlay and a committed age recipient (ADR-0202), and those are named
// with the same token everywhere rather than with a fourth spelling.
const localTier = "local"

// slugPattern is ADR-0003's charset rule, itself derived from RFC 1123 DNS labels.
var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// The project slug's bounds, including any collision token.
const (
	minProjectLen = 6
	maxProjectLen = 11
	maxSlugLen    = 63
)

// roleTableHeader matches the header row of the closed role table, whose two
// columns are the token and its meaning.
var roleTableHeader = regexp.MustCompile("^\\|\\s*`role`\\s*\\|\\s*Names\\s*\\|$")

// roleRow matches a row of the ADR's role table: `| `cp` | a Kubernetes … |`.
var roleRow = regexp.MustCompile("^\\|\\s*`([a-z0-9-]+)`\\s*\\|")

func main() {
	roles, err := parseRoles(adrFile)
	if err != nil {
		failf("%v", err)
	}
	if len(roles) == 0 {
		failf("%s: no role table found; the closed vocabulary is the gate's input", adrFile)
	}

	problems := make([]string, 0, len(roles))
	problems = append(problems, checkInventories(roles)...)
	problems = append(problems, checkSops()...)
	problems = append(problems, checkOverlays()...)

	if len(problems) > 0 {
		sort.Strings(problems)
		for _, p := range problems {
			_, _ = fmt.Fprintf(os.Stderr, "✗ %s\n", p)
		}
		_, _ = fmt.Fprintf(os.Stderr, "\n  Names derive from {project}-{env}-{role}[-{n}] (ADR-0003).\n")
		_, _ = fmt.Fprintf(os.Stderr, "  Roles: %s\n", strings.Join(sortedKeys(roles), ", "))
		os.Exit(1)
	}

	_, _ = fmt.Fprintln(os.Stdout, "✓ resource names follow {project}-{env}-{role}[-{n}]")
}

// parseRoles reads the closed role vocabulary out of the ADR's table. The table is
// the one whose rows are single backticked tokens followed by a prose column; the
// ADR's other tables have different shapes, and a row that does not match is simply
// not a role.
func parseRoles(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	roles := map[string]bool{}
	inTable := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// The table's own header, not the segment table's `role` ROW — which names
		// the same token and sits a few paragraphs above it.
		if roleTableHeader.MatchString(line) {
			inTable = true
			continue
		}
		if inTable {
			m := roleRow.FindStringSubmatch(line)
			if m == nil {
				// The table ended: a blank line, prose, or the header separator.
				if strings.HasPrefix(line, "| ---") {
					continue
				}
				inTable = false
				continue
			}
			roles[m[1]] = true
		}
	}
	err = scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return roles, nil
}

// checkInventories walks every Ansible inventory and parses each host name against
// the grammar. The env comes from the directory, so a host in inventory/dev naming
// itself `staging` is a finding rather than a matter of opinion.
func checkInventories(roles map[string]bool) []string {
	var problems []string

	entries, err := os.ReadDir(inventoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		failf("read %s: %v", inventoryDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		env := entry.Name()
		if !provisionedEnvs[env] {
			problem := fmt.Sprintf(
				"%s/%s: %q is not an environment (%s)",
				inventoryDir,
				env,
				env,
				strings.Join(sortedKeys(provisionedEnvs), ", "),
			)
			problems = append(problems, problem)
			continue
		}
		path := filepath.Join(inventoryDir, env, "hosts.yml")
		hosts, err := inventoryHosts(path)
		if err != nil {
			failf("%v", err)
		}
		// Every host in one inventory belongs to one project. A second slug is
		// either a typo or two projects sharing a control plane, and both want a
		// human to look.
		project := ""
		for _, host := range hosts {
			problem := checkHostName(host, env, roles)
			if problem != "" {
				problems = append(problems, path+": "+problem)
				continue
			}
			slug := strings.SplitN(host, "-", 2)[0]
			if project == "" {
				project = slug
			} else if slug != project {
				problem := fmt.Sprintf(
					"%s: host %q uses project slug %q, but this inventory is %q",
					path,
					host,
					slug,
					project,
				)
				problems = append(problems, problem)
			}
		}
	}
	return problems
}

// inventoryHosts reads the host keys out of an Ansible inventory, at whatever depth
// the groups nest them.
func inventoryHosts(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var root yaml.Node
	err = yaml.Unmarshal(data, &root)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var hosts []string
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil {
			return
		}
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == "hosts" && n.Content[i+1].Kind == yaml.MappingNode {
					for j := 0; j < len(n.Content[i+1].Content); j += 2 {
						hosts = append(hosts, n.Content[i+1].Content[j].Value)
					}
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&root)
	return hosts, nil
}

// checkHostName returns the problem with a host name, or "" when it is well formed.
func checkHostName(host, env string, roles map[string]bool) string {
	if !slugPattern.MatchString(host) {
		return fmt.Sprintf("host %q is not a slug (^[a-z][a-z0-9]*(-[a-z0-9]+)*$)", host)
	}
	if len(host) > maxSlugLen {
		return fmt.Sprintf(
			"host %q is %d characters; the bound is %d",
			host,
			len(host),
			maxSlugLen,
		)
	}

	parts := strings.Split(host, "-")
	if len(parts) < 3 {
		return fmt.Sprintf("host %q is not {project}-{env}-{role}[-{n}]", host)
	}
	project, gotEnv, role := parts[0], parts[1], parts[2]

	if len(project) < minProjectLen || len(project) > maxProjectLen {
		return fmt.Sprintf(
			"host %q: project slug %q is %d characters; the bound is %d–%d",
			host,
			project,
			len(project),
			minProjectLen,
			maxProjectLen,
		)
	}
	if gotEnv != env {
		return fmt.Sprintf(
			"host %q names env %q but lives in inventory/%s",
			host,
			gotEnv,
			env,
		)
	}
	if !roles[role] {
		return fmt.Sprintf("host %q: %q is not a role in ADR-0003's table", host, role)
	}
	// Only an ordinal may follow the role, and only where several of a role exist.
	if len(parts) > 4 {
		return fmt.Sprintf("host %q has more segments than {project}-{env}-{role}-{n}", host)
	}
	if len(parts) == 4 {
		_, err := strconv.Atoi(parts[3])
		if err != nil {
			return fmt.Sprintf(
				"host %q: %q is not an ordinal; names carry no descriptive suffix",
				host,
				parts[3],
			)
		}
	}
	return ""
}

// envToken matches the environment token wherever .sops.yaml names one: in an
// anchor (`&cluster_dev`) and in a path rule (`platform/dev/secrets`).
var envToken = regexp.MustCompile(`(?:&cluster_([a-z]+)|platform/([a-z]+)/secrets)`)

// checkSops asserts every environment .sops.yaml names is spelled out in full. A
// recipient anchor is where an abbreviation is most tempting and most damaging: the
// name is load-bearing for which key encrypts which file.
func checkSops() []string {
	data, err := os.ReadFile(sopsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		failf("read %s: %v", sopsFile, err)
	}

	var problems []string
	seen := map[string]bool{}
	for _, m := range envToken.FindAllStringSubmatch(string(data), -1) {
		env := m[1]
		if env == "" {
			env = m[2]
		}
		if seen[env] || provisionedEnvs[env] || env == localTier {
			continue
		}
		seen[env] = true
		problem := fmt.Sprintf(
			"%s: %q is not an environment; env is spelled out in full (%s, %s)",
			sopsFile,
			env,
			strings.Join(sortedKeys(provisionedEnvs), ", "),
			localTier,
		)
		problems = append(problems, problem)
	}
	return problems
}

// checkOverlays asserts the per-environment GitOps directories are named for the
// environments themselves. This is the surface ArgoCD's ApplicationSets select on,
// so a directory named `stg` becomes an env token in the cluster too.
func checkOverlays() []string {
	entries, err := os.ReadDir(overlayDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		failf("read %s: %v", overlayDir, err)
	}

	var problems []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if provisionedEnvs[name] || name == localTier {
			continue
		}
		problem := fmt.Sprintf(
			"%s/%s: %q is not an environment (%s, %s)",
			overlayDir,
			name,
			name,
			strings.Join(sortedKeys(provisionedEnvs), ", "),
			localTier,
		)
		problems = append(problems, problem)
	}
	return problems
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
