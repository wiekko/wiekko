#!/usr/bin/env just --justfile

set dotenv-load

TAILWIND := if os_family() == "windows" { "tailwindcss.exe" } else { "tailwindcss" }
TEMPL := if os_family() == "windows" { "templ.exe" } else { "templ" }

DIST := "web/dist"
STATIC := "web/static"

default:
    @just --list

all: clean generate copy-assets css build

build: generate css
    go vet ./...
    go build ./cmd/nexmap

clean:
    go clean
    rm -f nexmap nexmap.exe
    find . -name '*_templ.go' -delete
    rm -rf {{ DIST }}

css: copy-assets
    {{ TAILWIND }} -i {{ STATIC }}/css/input.css -o {{ DIST }}/css/styles.css --minify

copy-assets:
    @mkdir -p {{ DIST }}/css {{ DIST }}/fonts {{ DIST }}/js {{ DIST }}/img
    cp {{ STATIC }}/fonts/* {{ DIST }}/fonts/
    cp {{ STATIC }}/js/* {{ DIST }}/js/
    cp {{ STATIC }}/img/* {{ DIST }}/img/

dev:
    {{ TEMPL }} generate --watch &
    {{ TAILWIND }} -i {{ STATIC }}/css/input.css -o {{ DIST }}/css/styles.css --watch &
    go run ./cmd/nexmap

generate:
    {{ TEMPL }} generate

test:
    go test -v ./...
