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
internal/jobs         Job Hunter: searches, scoring, review queue, apply worker
internal/jobs/browser Chrome automation (go-rod): sessions, form reading/filling
internal/ai           LLM client: Anthropic, or any OpenAI-compatible API
internal/telegram     Telegram bot: notifications, buttons, replies
internal/secrets      AES-GCM sealing for stored keys and cookies
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

## Job Hunter

The dashboard's **Job Hunter** section searches LinkedIn, Indeed, Glints and
Kalibrr on a schedule, scores every new listing against the portfolio profile
with an LLM, and applies to the ones you approve.

1. **Search.** Each saved search has keywords, locations, remote/hybrid/on-site,
   job types, seniority, exclusions and a cron schedule (in the Job Hunter time
   zone). LinkedIn and Kalibrr are searched over their public endpoints; Glints
   and Indeed through Chrome.
2. **Score.** Hard filters run first (excluded words and companies, work mode,
   age, external-only apply, the same job already seen on another portal, a
   company applied to in the last 30 days). The rest get a 0–100 score with
   reasons. At or above *review-queue score* a job waits in the queue; with
   *auto-apply* on, at or above *auto-apply score* it is approved directly.
3. **Apply.** The worker opens the job in Chrome with your session, reads each
   form step, fills it from the profile, the answer bank and then the model,
   attaches the CV, and advances. A required question it can't answer stops the
   application and asks you (dashboard or Telegram reply); your answer is saved
   and reused. Every attempt keeps a screenshot.

Safety rails: **dry run is on by default** (forms are filled, Submit is never
pressed); applications only go out inside the daily window, with a random gap
per portal and per-portal and per-search daily caps; LinkedIn uses Easy Apply
only. The worker never solves CAPTCHAs: when a portal shows a human check it
pauses that portal and tells you, and you clear it once in the worker's Chrome
window. Automated applying is against these portals' terms of service and can
get an account restricted; keep the caps modest.

**Setup**

- Set `JOBS_SECRET_KEY` in `.env` (`openssl rand -hex 32`) and restart. It
  encrypts the AI keys, bot token and portal sessions stored in the database;
  every process sharing the database needs the same value, and changing it
  makes the stored secrets unreadable (re-enter them).
- *Job Hunter → Settings*: add an AI provider (Anthropic, OpenAI, Vikey,
  OpenRouter, Gemini, DeepSeek or any OpenAI-compatible URL), pick it for
  scoring and form filling, fill in the form facts (phone, expected salary…).
- *Accounts*: **Sign in on worker** opens Chrome on the worker machine for you
  to sign in (sessions are stored sealed; passwords never are), or paste
  cookies exported with an extension such as Cookie-Editor.
- Upload a CV under *CV*; forms attach it.
- *Searches → New search*, then **Run now** or wait for the schedule.

**Where the worker runs.** With `JOBS_WORKER=embedded` the server runs it and
needs Chrome on the same machine. Otherwise run `portfolio worker` with the same
`.env` on any machine that reaches the database and storage (a home connection
looks least like a bot); set `JOBS_WORKER=off` on the server. The distroless
Docker image has no Chrome, so use `JOBS_WORKER=off` there.

**Telegram.** Connect it in *Job Hunter → Settings → Telegram bot*: paste the
token from @BotFather, send the bot a message, press *Find my chat*. New matches arrive with ✅ Apply / ⏭ Skip buttons; questions
arrive as ❓ messages (tap a choice or reply with the answer). Commands:
`/status`, `/queue`, `/run`, `/pause`, `/resume`. A daily summary goes out at
the configured hour.

**Tests.** `go test ./...` runs the unit tests. The browser test drives the
real form engine through a mock multi-step application in Chrome; point it at a
disposable database: `PF_TEST_DATABASE_URL=postgres://… go test ./internal/jobs -run TestApplyFlow`.

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
