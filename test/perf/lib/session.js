// A signed-in session for the load scenarios (ADR-0304, ADR-0601).
//
// Writes are not anonymous any more: an order belongs to a buyer and to the org
// they act through, so `POST /api/orders` refuses a caller with no session. The
// suite cannot fake that with headers — the edge strips client-supplied identity
// headers before forward-auth, which is the whole point of them — so it logs in
// the way a browser does and carries the session cookie.
//
// The login runs once in `setup()`, not per iteration: it is not what the run is
// measuring, and one Kratos login per VU would put the identity plane in the
// numbers.
import http from "k6/http";

// login drives Kratos' browser login flow through the edge and returns the
// session cookie value.
//
// It throws rather than returning empty on any failure. k6 aborts the run when
// setup throws, which is the behaviour that matters here: a scenario that ran on
// 401s would report a healthy p95 for requests the platform never did.
export function login(baseURL, email, password) {
  const json = { headers: { Accept: "application/json", "Content-Type": "application/json" } };

  // Kratos mints the flow and answers with its UI descriptor. The CSRF cookie it
  // sets goes into this VU's jar and rides the POST below automatically.
  const init = http.get(`${baseURL}/auth/self-service/login/browser`, json);
  if (init.status !== 200) {
    throw new Error(`login: flow init at ${baseURL} returned HTTP ${init.status}`);
  }

  const flow = init.json();
  const action = flow?.ui?.action;
  if (!action) {
    throw new Error("login: flow has no ui.action — is the edge routing /auth/self-service?");
  }
  // The anti-CSRF token is a hidden node of the same form a browser would submit.
  const csrfNode = (flow.ui.nodes || []).find((n) => n?.attributes?.name === "csrf_token");
  const csrf = csrfNode?.attributes?.value;
  if (!csrf) {
    throw new Error("login: flow has no csrf_token node");
  }

  const res = http.post(
    action,
    JSON.stringify({ method: "password", identifier: email, password, csrf_token: csrf }),
    json,
  );
  if (res.status !== 200) {
    throw new Error(
      `login: ${email} got HTTP ${res.status} — has \`mise run auth:seed\` run against this cluster?`,
    );
  }

  const cookie = http.cookieJar().cookiesForURL(baseURL).ory_kratos_session;
  if (!cookie || cookie.length === 0) {
    throw new Error("login: succeeded but set no ory_kratos_session cookie");
  }
  return cookie[0];
}

// authHeaders is what a scenario adds to every request it makes as the buyer.
// Each VU has its own cookie jar and setup's jar is not shared with them, so the
// value is passed through setup's return and sent explicitly.
export function authHeaders(session) {
  return { Cookie: `ory_kratos_session=${session}` };
}
