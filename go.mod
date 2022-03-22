module github.com/algorand/indexer

go 1.16

replace github.com/algorand/go-algorand => ./third_party/go-algorand

require (
	github.com/algorand/go-algorand v0.0.0-20220322182955-997bd8641cbb
	github.com/algorand/go-algorand-sdk v1.9.1
	github.com/algorand/go-codec/codec v1.1.8
	github.com/algorand/oapi-codegen v1.3.7
	github.com/davecgh/go-spew v1.1.1
	github.com/getkin/kin-openapi v0.22.0
	github.com/jackc/pgconn v1.10.0
	github.com/jackc/pgerrcode v0.0.0-20201024163028-a0d42d470451
	github.com/jackc/pgx/v4 v4.13.0
	github.com/labstack/echo-contrib v0.11.0
	github.com/labstack/echo/v4 v4.3.0
	github.com/orlangure/gnomock v0.12.0
	github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_golang v1.10.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.7.1
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
)
