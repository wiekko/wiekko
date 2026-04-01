.PHONY: all clean generate css copy-assets build dev

TAILWIND := tailwindcss
TEMPL := templ
DIST := web/dist
STATIC := web/static

all: generate css copy-assets build

generate:
	$(TEMPL) generate

css: copy-assets
	$(TAILWIND) -i $(STATIC)/css/input.css -o $(DIST)/css/styles.css --minify

copy-assets:
	@mkdir -p $(DIST)/css $(DIST)/fonts $(DIST)/js $(DIST)/img
	cp $(STATIC)/fonts/* $(DIST)/fonts/
	cp $(STATIC)/js/* $(DIST)/js/
	cp $(STATIC)/img/* $(DIST)/img/

build:
	go build -o nexmap ./cmd/nexmap

clean:
	rm -rf $(DIST)
	find . -name '*_templ.go' -delete
	rm -f nexmap

dev:
	$(TEMPL) generate --watch &
	$(TAILWIND) -i $(STATIC)/css/input.css -o $(DIST)/css/styles.css --watch &
	go run ./cmd/nexmap
