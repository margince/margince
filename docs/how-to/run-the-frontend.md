<!-- prose:plain -->
# Run the frontend

The web app is in `frontend/`. It uses React 19 and Vite, and
[frontend/README.md](../../frontend/README.md) has more. It
calls only the `/v1` contract, and it has no other path past it.

## Develop

```sh
make dev   # full local stack, cold: db + migrate + the app on :8080 (api behind it)
```

`make dev` also starts the Vite dev server, and its `/v1` proxy points at
the API over HTTP. A browser counts `localhost` as safe, so the
`Secure` session cookie works without TLS. Open the app on
http://localhost:8080 and sign in to get the `crm_session` cookie. The
server finds its one company by itself, so you do not choose a
workspace. Stop the stack with `make dev-stop`.

## Check your work

```sh
make frontend-check   # Biome + unit tests + tsc + build (the frontend gate)
make frontend-e2e     # the screen-acceptance harness: AC-named tests,
                      # 390px sweep, axe WCAG 2.2 AA
make bench-mobile     # the perceived-perf budget, sampled on Fast-3G
```

By default, the `e2e` tests run with no network, against a stand-in backend that serves seed data.
To run the same tests against a live backend that holds seed data:

```sh
BASE_URL=http://localhost:8080 make frontend-e2e
```

## Generate the contract types again

After any change to `backend/api/crm.yaml`, run:

```sh
cd frontend && pnpm gen:api
```

A tool generates `src/api/schema.d.ts`, so never change it by hand.
