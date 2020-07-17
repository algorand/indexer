module github.com/algorand/indexer

go 1.13

require (
	github.com/algorand/go-algorand-sdk v1.4.2
	github.com/algorand/go-codec/codec v1.1.7
	github.com/algorand/oapi-codegen v1.3.5-algorand5
	github.com/getkin/kin-openapi v0.18.0
	github.com/labstack/echo/v4 v4.1.16
	github.com/lib/pq v1.3.0
	github.com/mattn/go-colorable v0.1.7 // indirect
	github.com/sirupsen/logrus v1.5.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.3
	github.com/stretchr/objx v0.1.1 // indirect
	github.com/stretchr/testify v1.5.1
	github.com/valyala/fasttemplate v1.2.0 // indirect
	golang.org/x/crypto v0.0.0-20200709230013-948cd5f35899 // indirect
	golang.org/x/net v0.0.0-20200707034311-ab3426394381 // indirect
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae // indirect
	golang.org/x/text v0.3.3 // indirect
)

replace github.com/algorand/go-algorand-sdk => github.com/algonautshant/go-algorand-sdk v0.0.0-20200708234359-21880c7a0d55
