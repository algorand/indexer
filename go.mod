module github.com/algorand/indexer

go 1.13

replace github.com/algorand/go-algorand => ../go-algorand

require (
	github.com/algorand/go-algorand v0.0.0-00010101000000-000000000000
	github.com/algorand/go-algorand-sdk v1.9.1
	github.com/algorand/go-codec/codec v1.1.7
	github.com/algorand/oapi-codegen v1.3.5-algorand5
	github.com/getkin/kin-openapi v0.22.0
	github.com/labstack/echo-contrib v0.11.0
	github.com/labstack/echo/v4 v4.3.0
	github.com/lib/pq v1.5.1
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/orlangure/gnomock v0.12.0
	github.com/prometheus/client_golang v1.10.0
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.7.0
	github.com/vektra/mockery v1.1.2 // indirect
)
