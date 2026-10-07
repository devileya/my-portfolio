# Arif Fadly Siregar — portfolio

A single Go binary that serves the whole site: the public portfolio (React),
a private dashboard for editing it, a JSON API, and a media proxy. Content lives
in **PostgreSQL**; images and the CV live in **Garage** (S3-compatible).

```
cmd/portfolio/        entry point: `portfolio` (serve) and `portfolio seed`
internal/config       env → Config
internal/db           pgx pool + embedded SQL migrations (run on start)
internal/storage      Garage/S3 client (private bucket, content-addressed keys)
internal/auth         admin login: bcrypt, JWT in an httpOnly SameSite=Strict cookie
internal/content      profile, projects, experience, skills, messages, counters
internal/server       routes, handlers, SPA + admin-path injection
internal/seed         first-run content loader (seed/data.json + seed/images)
web/                  React + TypeScript (Vite); built into web/dist and embedded
```

## The site

- **Hero** — the headline is "generated" token by token. Hover a word to see its
  probability, the alternatives, and what it attends to on the attention map.
  Raise the temperature and regenerate to sample a different headline. All
  headlines are editable in the dashboard (the first one is the T=0 answer).
- **Model card**, **work index** (filterable, expandable, hover previews),
  **experience**, **stack** (skill levels + tech frequency across projects),
  **CV download** (with size, date and SHA-256), and a **contact form** whose
  messages land in the dashboard inbox.
- **⌘K / Ctrl+K** opens a command menu.

## Dashboard

Open `ADMIN_PATH` (e.g. `https://your-domain/console-xxxxxxxx`). The path is
injected into the page by the server only for that URL, so it never appears in
the public JavaScript. From there you can edit the profile, upload the photo
and CV, manage projects/experience/skills (with ordering), read messages, and
change the password. The admin account is created on first start from
`ADMIN_EMAIL` / `ADMIN_PASSWORD`; after that the dashboard owns the password,
and changing it signs out every other session.

## Running locally

Requirements: Go 1.26+, Node 24+, a Postgres database.

```sh
cp .env.example .env           # fill in DATABASE_URL, S3_*, ADMIN_*, JWT_SECRET
make build                     # npm ci + vite build + go build
make seed CV=path/to/cv.pdf    # first run only: loads seed/data.json
./bin/portfolio                # http://localhost:8080
```

For front-end work, run `make dev-api` and `make dev-web` in two terminals and
open http://localhost:5173 (dashboard at http://localhost:5173/__admin).

## Garage

Uploads need a Garage key with read/write on the bucket. On the Garage host:

```sh
garage key create my-portfolio
garage bucket create my-portfolio
garage bucket allow --read --write --owner my-portfolio --key my-portfolio
garage key info my-portfolio --show-secret   # → S3_ACCESS_KEY / S3_SECRET_KEY
```

`S3_REGION` must match `s3_region` in `garage.toml`. The bucket stays private;
the app streams objects at `/media/...` with immutable caching. Without storage
the site still runs; uploads answer with a clear 503 and the dashboard says so.

If the database was seeded before storage was configured, re-run
`make seed FORCE=1 CV=path/to/cv.pdf` to attach the images and CV. This
replaces projects, experience and skills with the seed file, so do it before
editing content in the dashboard.

## Deploying

`docker build -t portfolio .` produces a distroless image with the binary and
seed content. Or copy `bin/portfolio` and a `.env` to the host and run it under
PM2/systemd behind nginx:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Real-IP $remote_addr;
    client_max_body_size 25m;    # CV uploads are capped at 20 MB
}
```

Set `APP_ENV=production` (Secure cookies, needs HTTPS) and `TRUST_PROXY=true`
behind nginx so rate limits see the real client IP.
