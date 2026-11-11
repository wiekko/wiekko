# Documentation site (docs.nexmap.net): mdBook pages for main and every release
# tag, served by nginx on port 8080. Build from the repository root:
#
#   docker build -f docs-site/Dockerfile -t nexmap-docs .   (or: just docs-container)
#
# The release tags come from .git in the build context. Without it (Coolify),
# build.sh clones the repository with a read-only token, passed as the BuildKit
# secret DOCS_GIT_TOKEN:
#
#   DOCS_GIT_TOKEN=... docker build --secret id=DOCS_GIT_TOKEN,env=DOCS_GIT_TOKEN ...
#
# See CONTRIBUTING.md, "Documentation site", and docs-site/DEPLOYMENT.md.

ARG BUILD_IMAGE=docker.io/library/alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
ARG RUNTIME_IMAGE=docker.io/nginxinc/nginx-unprivileged:1.31.7-alpine3.24@sha256:b5047b9b76845eac124358373c6852377c52e4b05834a47647e955dd91cb8f1e

FROM ${BUILD_IMAGE} AS build

ARG MDBOOK_VERSION=v0.5.4
ARG MDBOOK_SHA256_X64=5222beabd3e37dc5be0d18ff99b79058469354db5c220153a1b92db5ba12be89
ARG MDBOOK_SHA256_ARM64=753e5c5c363ee8a56972344dcf91466f005a51db84a7aeffe427ae3ef83d6d44

RUN apk add --no-cache git curl

# mdBook runs on the build machine, so it follows the build architecture.
RUN set -eu; \
    case "$(uname -m)" in \
        x86_64) target=x86_64-unknown-linux-musl; sum="$MDBOOK_SHA256_X64" ;; \
        aarch64) target=aarch64-unknown-linux-musl; sum="$MDBOOK_SHA256_ARM64" ;; \
        *) echo "unsupported build architecture: $(uname -m)" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/mdbook.tar.gz \
        "https://github.com/rust-lang/mdBook/releases/download/${MDBOOK_VERSION}/mdbook-${MDBOOK_VERSION}-${target}.tar.gz"; \
    echo "${sum}  /tmp/mdbook.tar.gz" | sha256sum -c -; \
    tar -xzf /tmp/mdbook.tar.gz -C /usr/local/bin mdbook; \
    rm /tmp/mdbook.tar.gz

WORKDIR /src
COPY . .

# Where to clone from when the context has no .git; build.sh has the default.
ARG DOCS_GIT_URL=""

# The token is only accepted as a secret, mounted at /run/secrets/DOCS_GIT_TOKEN,
# never as a build argument: those end up in the image's build history.
# The context's files belong to another user than the one running git here.
RUN --mount=type=secret,id=DOCS_GIT_TOKEN \
    git config --global --add safe.directory /src \
    && sh docs-site/build.sh /site

FROM ${RUNTIME_IMAGE} AS runtime

COPY docs-site/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /site /usr/share/nginx/html

EXPOSE 8080
