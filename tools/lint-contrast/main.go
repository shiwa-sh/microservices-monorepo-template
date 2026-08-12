// Command lint-contrast checks colour contrast against the design-token file
// rather than per component (ADR-0400).
//
// The reason it is not a per-component check: a token change moves every surface
// at once, so the token file is where a contrast regression is introduced and where
// it is cheapest to catch. axe still scans the rendered pages (test/e2e), and that
// catches composition mistakes this cannot see — a foreground applied over a
// background the naming convention does not pair it with.
//
// The pairing comes from the token names themselves, which is what makes this
// mechanical rather than a hand-maintained list:
//
//	--color-text-*            over  --color-bg-primary
//	--color-text-*_on-brand   over  --color-bg-brand-solid
//
// Thresholds follow the success criteria rather than the visual hierarchy. SC 1.4.3
// applies to ALL text at 4.5:1 — a token named `quaternary` or `placeholder` is
// still text, and low prominence is not an exception the criterion grants. Icon
// fills are non-text and take SC 1.4.11's 3:1. Disabled states are the one genuine
// exemption 1.4.3 names: an inactive user-interface component is incidental.
//
// Both the light palette and the .dark-mode block are checked. A theme that only
// conforms in one mode conforms in neither, since the user picks.
//
// A pair that cannot be resolved to two concrete colours is a hard failure, never a
// skip. The first version of this tool skipped them, and it silently checked three
// pairs out of forty-five because `--color-white` is supplied by Tailwind's own
// theme rather than declared here — a gate that reports success while measuring
// nothing is worse than no gate.
//
// The neutral, red, green, and yellow ramps the theme aliases are Tailwind's, in
// oklch(). They are read from the installed Tailwind theme and layered underneath
// the project's own declarations, which override them.
package main

import (
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const themeFile = "apps/frontend/src/styles/theme.css"

// Tailwind ships its palette as CSS custom properties. Bun's store nests the real
// package under .bun/, so both layouts are searched.
var tailwindThemeGlobs = []string{
	"node_modules/tailwindcss/theme.css",
	"node_modules/.bun/tailwindcss@*/node_modules/tailwindcss/theme.css",
	"apps/frontend/node_modules/tailwindcss/theme.css",
}

const (
	thresholdText = 4.5 // WCAG 2.2 SC 1.4.3, normal text
	thresholdUI   = 3.0 // WCAG 2.2 SC 1.4.11, non-text contrast
)

// Icon fills are non-text and take SC 1.4.11's 3:1 threshold.
var nonTextMarkers = []string{"icon"}

// Disabled states are incidental under SC 1.4.3's own exception for an inactive
// user-interface component, so they are not scored at all.
var exemptMarkers = []string{"disabled"}

type rgb struct{ r, g, b float64 }

// CSS-universal colours Tailwind declares as keywords rather than values.
var baseColors = map[string]rgb{
	"--color-white": {1, 1, 1},
	"--color-black": {0, 0, 0},
}

// A token named for a colour states its own value rather than a role, so there is
// no ground to infer for it: --color-text-white is white text for a dark surface,
// and pairing it with the page background would measure white on white.
var selfColoured = []string{"--color-text-white", "--color-text-black"}

var (
	declRe  = regexp.MustCompile(`(?m)^\s*(--color-[a-z0-9-_]+)\s*:\s*([^;]+);`)
	varRe   = regexp.MustCompile(`var\(\s*(--[a-z0-9-_]+)\s*\)`)
	rgbRe   = regexp.MustCompile(`rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)`)
	oklchRe = regexp.MustCompile(`oklch\(\s*([0-9.]+)%?\s+([0-9.]+)\s+([0-9.]+)`)
	hexRe   = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
)

func main() {
	data, err := os.ReadFile(themeFile)
	if err != nil {
		failf("read %s: %v", themeFile, err)
	}
	base, err := tailwindPalette()
	if err != nil {
		failf("%v", err)
	}
	light, dark := split(string(data))

	var problems []string
	checked := 0
	for _, mode := range []struct {
		name   string
		tokens map[string]string
	}{
		{"light", layer(base, parse(light))},
		{"dark", layer(base, parse(dark))},
	} {
		if len(mode.tokens) == 0 {
			failf("%s: no colour tokens found in the %s palette", themeFile, mode.name)
		}
		found, probs := checkMode(mode.name, mode.tokens)
		checked += found
		problems = append(problems, probs...)
	}

	if len(problems) > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "✗ design tokens fail WCAG 2.2 AA contrast (ADR-0400):\n")
		for _, p := range problems {
			_, _ = fmt.Fprintln(os.Stderr, "  "+p)
		}
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "✓ %d token pairs meet WCAG 2.2 AA contrast\n", checked)
}

// checkMode pairs every text token with its background and returns the failures.
func checkMode(mode string, tokens map[string]string) (int, []string) {
	var problems []string
	checked := 0

	for name := range tokens {
		if !strings.HasPrefix(name, "--color-text-") || slices.Contains(selfColoured, name) {
			continue
		}
		bgName := "--color-bg-primary"
		if strings.HasSuffix(name, "_on-brand") {
			bgName = "--color-bg-brand-solid"
		}

		if isExempt(name) {
			continue
		}

		fg, okFG := resolve(tokens, name)
		if !okFG {
			problems = append(problems, fmt.Sprintf("%s: %s does not resolve to a colour", mode, name))
			continue
		}
		bg, okBG := resolve(tokens, bgName)
		if !okBG {
			problems = append(problems, fmt.Sprintf("%s: %s does not resolve to a colour", mode, bgName))
			continue
		}

		want := thresholdText
		criterion := "1.4.3 normal text"
		if isNonText(name) {
			want = thresholdUI
			criterion = "1.4.11 non-text"
		}

		checked++
		got := contrast(fg, bg)
		if got < want {
			const form = "%s: %s on %s is %.2f:1, below %.1f:1 (SC %s)"
			problems = append(problems, fmt.Sprintf(form, mode, name, bgName, got, want, criterion))
		}
	}
	return checked, problems
}

func isExempt(name string) bool {
	for _, m := range exemptMarkers {
		if strings.Contains(name, m) {
			return true
		}
	}
	return false
}

func isNonText(name string) bool {
	for _, m := range nonTextMarkers {
		if strings.Contains(name, m) {
			return true
		}
	}
	return false
}

// tailwindPalette reads the ramps the theme aliases but does not declare.
func tailwindPalette() (map[string]string, error) {
	for _, pattern := range tailwindThemeGlobs {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", pattern, err)
		}
		slices.Sort(matches)
		for _, m := range slices.Backward(matches) {
			data, err := os.ReadFile(m)
			if err != nil {
				continue
			}
			return parse(string(data)), nil
		}
	}
	return nil, fmt.Errorf("no Tailwind theme.css found — run `bun install` (searched %v)", tailwindThemeGlobs)
}

// layer puts the project's declarations over Tailwind's, which is the cascade the
// browser sees: globals.css imports tailwindcss and then theme.css.
func layer(base, over map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(over))
	maps.Copy(out, base)
	maps.Copy(out, over)
	return out
}

// split separates the light palette from the .dark-mode override block. The dark
// palette is the light one with the overrides applied, because .dark-mode only
// restates what changes.
func split(css string) (string, string) {
	idx := strings.Index(css, ".dark-mode")
	if idx < 0 {
		return css, css
	}
	light := css[:idx]
	return light, light + css[idx:]
}

// parse collects every --color-* declaration. A later declaration wins, which is
// what makes the dark block override the light one.
func parse(css string) map[string]string {
	out := map[string]string{}
	for _, m := range declRe.FindAllStringSubmatch(css, -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

// resolve follows var() indirection to a concrete colour. The depth cap is a cycle
// guard: a token file with a reference loop should fail the lint, not hang it.
func resolve(tokens map[string]string, name string) (rgb, bool) {
	c, ok := baseColors[name]
	if ok {
		return c, true
	}
	value, declared := tokens[name]
	if !declared {
		return rgb{}, false
	}
	for range 16 {
		m := varRe.FindStringSubmatch(value)
		if m == nil {
			return parseColor(value)
		}
		base, isBase := baseColors[m[1]]
		if isBase {
			return base, true
		}
		next, found := tokens[m[1]]
		if !found {
			return rgb{}, false
		}
		value = strings.TrimSpace(next)
	}
	return rgb{}, false
}

func parseColor(value string) (rgb, bool) {
	value = strings.TrimSpace(value)
	m := rgbRe.FindStringSubmatch(value)
	if m != nil {
		r, _ := strconv.ParseFloat(m[1], 64)
		g, _ := strconv.ParseFloat(m[2], 64)
		b, _ := strconv.ParseFloat(m[3], 64)
		return rgb{r / 255, g / 255, b / 255}, true
	}
	m = oklchRe.FindStringSubmatch(value)
	if m != nil {
		l, _ := strconv.ParseFloat(m[1], 64)
		c, _ := strconv.ParseFloat(m[2], 64)
		h, _ := strconv.ParseFloat(m[3], 64)
		if strings.Contains(value, "%") {
			l /= 100
		}
		return oklchToSRGB(l, c, h), true
	}
	if hexRe.MatchString(value) {
		h := strings.TrimPrefix(value, "#")
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		n, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return rgb{}, false
		}
		return rgb{
			float64((n>>16)&0xff) / 255,
			float64((n>>8)&0xff) / 255,
			float64(n&0xff) / 255,
		}, true
	}
	return rgb{}, false
}

// oklchToSRGB converts OKLCH to gamma-encoded sRGB. Tailwind publishes its palette
// in OKLCH, and WCAG's contrast formula is defined on sRGB, so the conversion is
// unavoidable rather than a preference.
func oklchToSRGB(lightness, chroma, hue float64) rgb {
	rad := hue * math.Pi / 180
	a := chroma * math.Cos(rad)
	b := chroma * math.Sin(rad)

	lp := lightness + 0.3963377774*a + 0.2158037573*b
	mp := lightness - 0.1055613458*a - 0.0638541728*b
	sp := lightness - 0.0894841775*a - 1.2914855480*b
	l, m, sc := lp*lp*lp, mp*mp*mp, sp*sp*sp

	lr := 4.0767416621*l - 3.3077115913*m + 0.2309699292*sc
	lg := -1.2684380046*l + 2.6097574011*m - 0.3413193965*sc
	lb := -0.0041960863*l - 0.7034186147*m + 1.7076147010*sc
	return rgb{gamma(lr), gamma(lg), gamma(lb)}
}

// gamma encodes a linear sRGB channel and clamps out-of-gamut results, which OKLCH
// can produce for saturated colours.
func gamma(v float64) float64 {
	if v <= 0.0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	return math.Min(1, math.Max(0, v))
}

// relativeLuminance is WCAG 2.x's definition, not perceptual lightness.
func relativeLuminance(c rgb) float64 {
	lin := func(v float64) float64 {
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

func contrast(fg, bg rgb) float64 {
	l1, l2 := relativeLuminance(fg), relativeLuminance(bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
