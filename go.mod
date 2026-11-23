module github.com/nerdneilsfield/aumbry

go 1.27.1

require (
	github.com/abema/go-mp4 v1.7.4
	github.com/aws/aws-sdk-go-v2 v1.48.0
	github.com/aws/aws-sdk-go-v2/credentials v1.21.3
	github.com/aws/aws-sdk-go-v2/service/s3 v1.116.0
	github.com/aws/smithy-go v1.28.9
	github.com/caarlos0/env/v11 v11.4.2
	github.com/gabriel-vasile/mimetype v1.4.15
	github.com/go-chi/chi/v5 v5.3.0
	github.com/go-chi/httplog/v3 v3.5.0
	github.com/go-chi/httprate v0.16.1
	github.com/gofrs/flock v0.13.1
	github.com/gorilla/securecookie v1.1.2
	github.com/jmoiron/sqlx v1.4.0
	github.com/johannesboyne/gofakes3 v1.2.0
	github.com/lestrrat-go/helium v0.11.0
	github.com/matoous/go-nanoid/v2 v2.1.0
	github.com/pelletier/go-toml/v2 v2.4.3
	github.com/pressly/goose/v3 v3.24.1
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	github.com/spf13/cobra v1.10.2
	github.com/studio-b12/gowebdav v0.10.0
	github.com/tcolgate/mp3 v0.0.0-20170426193717-e79c5a46d300
	github.com/unrolled/secure v1.17.0
	go.uber.org/zap v1.28.0
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/crypto v0.59.0
	golang.org/x/net v0.62.0
	golang.org/x/sync v0.25.0
	modernc.org/sqlite v1.34.5
)

tool (
	github.com/evilmartians/lefthook/v2
	golang.org/x/tools/cmd/goimports
	mvdan.cc/gofumpt
)

require (
	al.essio.dev/pkg/shellescape v1.6.1 // indirect
	charm.land/lipgloss/v2 v2.0.6 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.27 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.12 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.12 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.12 // indirect
	github.com/bmatcuk/doublestar/v4 v4.10.2 // indirect
	github.com/briandowns/spinner v1.23.2 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260811164956-006e29f97886 // indirect
	github.com/charmbracelet/x/ansi v0.11.8 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/evilmartians/lefthook/v2 v2.2.2 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kaptinlin/jsonpointer v0.4.28 // indirect
	github.com/kaptinlin/jsonschema v0.9.10 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/knadh/koanf/maps v0.1.3 // indirect
	github.com/knadh/koanf/parsers/json v1.0.1 // indirect
	github.com/knadh/koanf/parsers/toml/v2 v2.2.2 // indirect
	github.com/knadh/koanf/parsers/yaml v1.1.1 // indirect
	github.com/knadh/koanf/providers/fs v1.0.1 // indirect
	github.com/knadh/koanf/providers/rawbytes v1.0.1 // indirect
	github.com/knadh/koanf/v2 v2.3.7 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.30 // indirect
	github.com/mattn/go-tty v0.0.8 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/ryszard/goskiplist v0.0.0-20150312221310-2dfbae5fcf46 // indirect
	github.com/schollz/progressbar/v3 v3.19.1 // indirect
	github.com/sethvargo/go-retry v0.3.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tidwall/jsonc v0.3.3 // indirect
	github.com/urfave/cli/v3 v3.14.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	go.shabbyrobe.org/gocovmerge v0.0.0-20230507111327-fa4f82cfbf4d // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/telemetry v0.0.0-20261009151841-97e8ea77aa2a // indirect
	golang.org/x/term v0.48.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	modernc.org/libc v1.55.3 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
	mvdan.cc/gofumpt v0.12.0 // indirect
)
