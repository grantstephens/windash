[![CC BY-NC-SA 4.0][cc-by-nc-sa-image]][cc-by-nc-sa]

[cc-by-nc-sa]: http://creativecommons.org/licenses/by-nc-sa/4.0/
[cc-by-nc-sa-image]: https://licensebuttons.net/l/by-nc-sa/4.0/88x31.png

# Tools Needed

* [Fastly CLI](https://github.com/fastly/cli)
* [Go](https://go.dev/doc/install)

# Getting Started

1. Add API Key to secret.json file
2. Run `make dev`
3. Go to [127.0.0.1:7676](http://127.0.0.1:7676)

# What's going on?

* `main.go` is a mess, but generally renderes the `index.html.tmpl`
* API calls are cached or saved in the KV store.
* Deploys happen automaticall when pushed to main branch.
* Dev uses `go` but in production tinygo is used (See difference between fastly.toml and fastly.dev.toml). This is importaint because not everything works in tinygo (like `encoding/json`) and tinygo results in a smaller binary.

# TODO
* Tests
* Yearly data + Plots
