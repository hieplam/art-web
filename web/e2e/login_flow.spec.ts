// web/e2e/login_flow.spec.ts
//
// Two complementary tests for the auth surface:
//
//   1. Click-and-redirect: clicking "Sign in" must produce a request to
//      Google's /o/oauth2/v2/auth with a populated client_id and the
//      correct redirect_uri. This is the regression test for the original
//      "Missing required parameter: client_id" bug.
//
//   2. Post-auth state: when the alice JWT cookie (issued by /dev/seed) is
//      set on the web origin, the Nav must drop the Sign-in CTA and render
//      the user's profile link. This tests the SSR cookie-forwarding path
//      end-to-end without involving Google at all.
//
// Neither test calls real Google. Test 1 stubs the /o/oauth2/** response
// with `context.route` so we never leave localhost; test 2 sidesteps the
// OAuth flow entirely by reusing the seed-issued JWT.
import { test, expect } from "@playwright/test";
import { seedMatrix } from "./_helpers";

test("sign-in button initiates Google OAuth with valid client_id and redirect_uri", async ({
  page,
}) => {
  // Block the actual network call to Google so the test never leaves localhost
  // (and so it works in CI where accounts.google.com may be unreachable).
  // page.route (not context.route) is more reliable for top-level navigation
  // interception, and a regex pattern matches the full URL including any
  // protocol/path quirks.
  await page.route(/accounts\.google\.com/, (route) =>
    route.fulfill({
      status: 200,
      contentType: "text/html",
      body: "<html><body><h1>mocked-google-consent</h1></body></html>",
    }),
  );

  await page.goto("/");

  // Subscribe to the request before triggering it — waitForRequest returns the
  // captured Request directly, no closure plumbing needed.
  const googleReqPromise = page.waitForRequest(/accounts\.google\.com\/o\/oauth2/);
  await page.locator("a.signin").click();
  const googleReq = await googleReqPromise;

  const url = new URL(googleReq.url());
  const params = url.searchParams;

  // Regression for the original bug: client_id MUST be a non-empty string.
  // If GOOGLE_OAUTH_CLIENT_ID isn't wired into the API container, this will
  // be empty and the assertion fails — exactly the failure mode we hit in
  // the user-facing flow.
  const clientID = params.get("client_id");
  expect(clientID, "client_id missing from authorize URL — env not wired?").toBeTruthy();
  expect(clientID).not.toBe("");

  expect(params.get("redirect_uri")).toBe("http://localhost:8080/auth/google/callback");
  expect(params.get("response_type")).toBe("code");
  expect(params.get("scope")).toBe("openid email profile");
  expect(params.get("state")).toMatch(/^[a-f0-9]{32}$/);
});

test("nav shows signed-in user when auth cookie is set", async ({ page, context }) => {
  const seed = await seedMatrix();

  // The seed endpoint returns aliceCookie shaped like "auth=<jwt>" (matching
  // the Set-Cookie value the real CallbackHandler writes). Strip the name
  // and any trailing attributes for Playwright's addCookies API.
  const jwt = seed.aliceCookie.replace(/^auth=/, "").split(/[;\s]/)[0];
  expect(jwt, "seed did not return a JWT for alice").toBeTruthy();

  await context.addCookies([
    {
      name: "auth",
      value: jwt,
      url: "http://localhost:3000",
    },
  ]);

  await page.goto("/");

  // Signed-in: Sign-in link is gone, profile link is shown.
  await expect(page.locator("nav a.signin")).toHaveCount(0);
  await expect(page.locator(`nav a[href="/u/${seed.aliceSlug}"]`)).toBeVisible();
});
