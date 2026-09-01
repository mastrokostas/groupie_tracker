# Plan: deploying two projects side by side

## What exists today

One AWS EC2 box in `eu-north-1` (Stockholm), Elastic IP `16.16.87.155`, Ubuntu with Docker Engine and Docker Compose.

Two containers run on it, both started by a single `docker-compose.yml` in `/home/ubuntu/ascii-art-web`:

- the ascii-art-web Go app, image `ghcr.io/mastrokostas/ascii-art-web:latest`, listening on 8080, not published to the host
- nginx, image `ghcr.io/mastrokostas/ascii-art-web/nginx:latest`, publishing 80 and 443, terminating TLS and proxying inward to `app:8080`

Cloudflare sits in front on Full SSL. A Cloudflare Origin certificate covering `*.mastrokostas.com` lives on the box at `/home/ubuntu/ascii-art-web/certs` and is bind-mounted into the nginx container read-only. It is not in any repository.

One GitHub Actions pipeline in the ascii-art-web repository does test, build-and-push, smoke-test, deploy. The deploy job copies `docker-compose.yml` to the box over SCP, then over SSH pulls images and runs `docker compose up -d`.

## What we are building

Three containers on the same box:

1. **nginx plus a start page.** Serves a landing page and reverse-proxies to the two apps.
2. **ascii-art-web.** Unchanged Go app.
3. **groupie-tracker.** The second Go project, deployed for the first time.

Three separate GitHub repositories, one per container, each with its own pipeline.

Routing is by subdomain, not by URL path. Each app keeps its own root, so no application code changes are needed in either Go project.

## Routing

nginx splits traffic on `server_name`. Three server blocks on port 443, plus the existing port 80 block that redirects everything to HTTPS.

Rough shape:

```
server {
    server_name ascii-art-web.mastrokostas.com;
    location / { proxy_pass http://ascii_art_web:8080; }
}

server {
    server_name groupie-tracker.mastrokostas.com;
    location / { proxy_pass http://groupie_tracker:8080; }
}

server {
    server_name <start page hostname — not decided yet>;
    root /usr/share/nginx/html;
}
```

The existing `proxy_set_header` lines carry over unchanged.

Because each app answers at its own root, absolute paths in the HTML (`/static/style.css`, `/ascii-art`, redirects to `/`) resolve correctly with no modification. This is the reason subdomains were chosen over paths.

## DNS and certificates

Add A records in Cloudflare pointing at `16.16.87.155`, proxied, one per subdomain. The exact hostnames are listed as a gap below.

No certificate work. The `*.mastrokostas.com` origin certificate already covers every subdomain, and it is already on the box.

## The compose file — the one real problem

The compose file names all three images and defines all three services. Only one place can own it, or two pipelines will overwrite each other and delete containers they do not own.

Two options. **Pick one before writing any workflow files.**

**Option A — the box owns it.** The compose file lives only in `/home/ubuntu/<directory>` on the server, placed there by hand. No pipeline copies it. Each of the three deploy jobs does only:

```
docker compose pull <service name>
docker compose up -d <service name>
```

Naming the service matters. Without it, a deploy of one project restarts all three containers.

Simple, and it works. The cost is that the compose file is not in version control.

**Option B — a fourth repository owns it.** An infrastructure repository holds `docker-compose.yml`. The three app repositories only build and push images; they never copy the compose file. Deploying a change to the compose file is a push to the infrastructure repo.

More moving parts, but the compose file is versioned like everything else.

## Per-repository pipelines

Each of the three repositories gets the same four-job shape already in use: test, build-and-push, smoke-test, deploy. Same deploy SSH key, same three secrets (`DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_SSH_KEY`) copied into each repository.

Two differences from the existing pipeline:

**Deploy job.** Restricted to its own service name, as described above.

**Smoke test for the nginx repository.** The current smoke test starts the image alone and curls it. The nginx image cannot start alone — it tries to resolve `ascii_art_web` and `groupie_tracker` as proxy targets, fails, and exits. This job needs modification. The approach is not decided yet; see gaps.

The ascii-art-web and groupie-tracker smoke tests work as they are: start the image, curl `/` for a 200, curl a nonexistent path for a 404.

## Order of work

1. Decide compose file ownership (Option A or B).
2. Decide subdomain hostnames and start page hostname.
3. Create the nginx repository. Move `nginx/` out of the ascii-art-web repository into it, add the start page, add the two new server blocks.
4. Create the groupie-tracker repository if it does not already exist, add a Dockerfile.
5. Write the three-service compose file, put it wherever step 1 decided.
6. Write the three pipelines.
7. Add the Cloudflare DNS records.
8. Remove the nginx build steps and the nginx service from the ascii-art-web repository's workflow, leaving it responsible for its own image only.

## Gaps — do not fill these in, ask

- Compose file ownership: Option A or Option B. Undecided.
- The exact hostname for the start page.
- The exact subdomain hostnames for the two projects (assumed above to be `ascii-art-web.mastrokostas.com` and `groupie-tracker.mastrokostas.com`, but not confirmed).
- The groupie-tracker repository name and its GHCR image name.
- The port groupie-tracker listens on. Assumed 8080 above, not confirmed.
- Whether groupie-tracker has a Dockerfile already.
- What the start page contains, and whether it is static HTML in the nginx image or something else.
- How the nginx smoke test should be modified.
- The deploy directory name on the box, if it changes from `/home/ubuntu/ascii-art-web` now that it holds three projects.
