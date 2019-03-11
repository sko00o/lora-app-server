module github.com/brocaar/lora-app-server

require (
	cloud.google.com/go v0.36.0
	github.com/Azure/azure-service-bus-go v0.2.0
	github.com/NickBall/go-aes-key-wrap v0.0.0-20170929221519-1c3aa3e4dfc5
	github.com/aws/aws-sdk-go v1.17.5
	github.com/brocaar/loraserver v0.0.0-20190116145810-3cdb0c99d7e3
	github.com/brocaar/lorawan v0.0.0-20190305110132-11ffaf662692
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/eclipse/paho.mqtt.golang v0.0.0-20190117150808-cb7eb9363b44
	github.com/elazarl/go-bindata-assetfs v0.0.0-20180223160309-38087fe4dafb
	github.com/gobuffalo/packr v1.22.0 // indirect
	github.com/gofrs/uuid v3.2.0+incompatible
	github.com/gogo/protobuf v1.2.1 // indirect
	github.com/golang/protobuf v1.3.0
	github.com/gomodule/redigo v2.0.0+incompatible
	github.com/gopherjs/gopherjs v0.0.0-20181103185306-d547d1d9531e // indirect
	github.com/goreleaser/goreleaser v0.101.0
	github.com/goreleaser/nfpm v0.9.7
	github.com/gorilla/mux v1.7.0
	github.com/gorilla/websocket v1.4.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v0.0.0-20190104160321-4832df01553a
	github.com/grpc-ecosystem/grpc-gateway v1.8.1
	github.com/jmoiron/sqlx v1.2.0
	github.com/jteeuwen/go-bindata v3.0.8-0.20180305030458-6025e8de665b+incompatible
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/lib/pq v1.0.0
	github.com/mattn/go-colorable v0.1.1 // indirect
	github.com/mattn/go-isatty v0.0.6 // indirect
	github.com/mmcloughlin/geohash v0.0.0-20181009053802-f7f2bcae3294
	github.com/pkg/errors v0.8.1
	github.com/robertkrimen/otto v0.0.0-20180617131154-15f95af6e78d
	github.com/rubenv/sql-migrate v0.0.0-20181213081019-5a8808c14925
	github.com/sirupsen/logrus v1.3.0
	github.com/smartystreets/assertions v0.0.0-20190215210624-980c5ac6f3ac // indirect
	github.com/smartystreets/goconvey v0.0.0-20190222223459-a17d461953aa
	github.com/smartystreets/gunit v0.0.0-20180314194857-6f0d6275bdcd // indirect
	github.com/spf13/cobra v0.0.3
	github.com/spf13/viper v1.3.1
	github.com/stretchr/testify v1.3.0
	github.com/tmc/grpc-websocket-proxy v0.0.0-20190109142713-0ad062ec5ee5
	github.com/ziutek/mymysql v1.5.4 // indirect
	go.opencensus.io v0.19.0 // indirect
	golang.org/x/crypto v0.0.0-20190228161510-8dd112bcdc25
	golang.org/x/lint v0.0.0-20190301231843-5614ed5bae6f
	golang.org/x/net v0.0.0-20190301231341-16b79f2e4e95
	golang.org/x/oauth2 v0.0.0-20190226205417-e64efc72b421 // indirect
	golang.org/x/sys v0.0.0-20190305064518-30e92a19ae4a // indirect
	golang.org/x/tools v0.0.0-20190228203856-589c23e65e65
	google.golang.org/api v0.1.0
	google.golang.org/genproto v0.0.0-20190227213309-4f5b463f9597
	google.golang.org/grpc v1.19.0
	gopkg.in/gorp.v1 v1.7.2 // indirect
	gopkg.in/sourcemap.v1 v1.0.5 // indirect
)

replace github.com/grpc-ecosystem/grpc-gateway => github.com/brocaar/grpc-gateway v1.7.0-patched
