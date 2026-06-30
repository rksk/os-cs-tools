// Local-development runtime config for the CSM portal FE, pointing at a
// LOCALLY-RUN csm-portal-backend (BFF) instead of the staging Choreo backend.
//
// The FE never calls entity-service directly: all entity queries go through the
// BFF, which proxies to entity-service server-side. So "local BE" here means a
// local csm-portal-backend (default :8080); that backend is what you point at a
// local entity-service.
//
// ── How to use ────────────────────────────────────────────────────────────
// index.html loads "/config.js" (which is gitignored). To activate this file:
//
//     cp public/config.local.js public/config.js
//     pnpm dev
//
// Then run the BFF locally on :8080 (and the entity-service it calls).
//
// ── Auth caveat (read this) ─────────────────────────────────────────────────
// In production a gateway validates the OIDC token and forwards it to the BFF
// as the `x-jwt-assertion` header. Calling the BFF DIRECTLY from the browser
// (as this file does) skips that step, so the BFF must accept the FE's auth in
// local mode. If it does not, prefer the built-in same-origin gateway mode,
// which proxies /local-api → the BFF and maps Authorization: Bearer →
// x-jwt-assertion for you, with a mock IdP:
//
//     CSM_PORTAL_LOCAL_BE=1 pnpm dev      # see vite.config.ts
//
// That mode overrides this config at serve time, so you don't need this file
// for it — this file is for the "directly reachable local BFF" case.
window.config = {
  // OIDC IdP. Keep your real Asgardeo tenant if the local BFF validates real
  // tokens; switch to the local mock IdP (http://localhost:9100) only when
  // running the full local stack.
  CSM_PORTAL_AUTH_BASE_URL: "https://api.asgardeo.io/t/wso2",
  CSM_PORTAL_AUTH_CLIENT_ID: "Yezn9A7Jb2ROuApr4m5hXHflgAka",
  CSM_PORTAL_AUTH_SIGN_IN_REDIRECT_URL: "http://localhost:3001",
  CSM_PORTAL_AUTH_SIGN_OUT_REDIRECT_URL: "http://localhost:3001",

  // Local csm-portal-backend (BFF). Change the port if you run it elsewhere.
  CSM_PORTAL_BACKEND_BASE_URL: "http://localhost:8080",

  CSM_PORTAL_THEME: "acrylicOrange",
  CSM_PORTAL_LOG_LEVEL: "DEBUG",

  // Use the live (local) backend, not fixtures. Toggle left on so you can flip
  // back to mocks in-header if the local BFF is down.
  CSM_PORTAL_USE_MOCKS: false,
  CSM_PORTAL_ALLOW_MOCK_TOGGLE: true,

  CSM_PORTAL_MAINTENANCE_BANNER_VISIBLE: false,
  CSM_PORTAL_TOP_BANNER_ENABLED: false,
};
