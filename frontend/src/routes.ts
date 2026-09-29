// The app is served under /ui by the Go server. The router is declared from
// these segments (App.tsx) and every link and redirect uses the paths built
// from them, so a path can't drift between the two.
export const ROUTE_SEGMENTS = {
  base: 'ui',
  login: 'login',
  callback: 'callback',
  streets: 'streets',
} as const;

const BASE = `/${ROUTE_SEGMENTS.base}`;

export const ROUTES = {
  root: BASE,
  login: `${BASE}/${ROUTE_SEGMENTS.login}`,
  callback: `${BASE}/${ROUTE_SEGMENTS.callback}`,
  streets: `${BASE}/${ROUTE_SEGMENTS.streets}`,
  street: (streetId: string) => `${BASE}/${ROUTE_SEGMENTS.streets}/${streetId}`,
} as const;
