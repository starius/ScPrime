module gitlab.com/scpcorp/ScPrime

go 1.20

replace gitlab.com/scpcorp/spf-transporter => /home/user/go/src/gitlab.com/scpcorp/spf-transporter

require (
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da
	github.com/dchest/threefish v0.0.0-20120919164726-3ecf4c494abf
	github.com/go-sql-driver/mysql v1.7.1
	github.com/google/gofuzz v1.0.0
	github.com/hanwen/go-fuse/v2 v2.3.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/inconshreveable/go-update v0.0.0-20160112193335-8152e7eb6ccf
	github.com/julienschmidt/httprouter v1.3.0
	github.com/klauspost/reedsolomon v1.11.7
	github.com/sasha-s/go-deadlock v0.3.1
	github.com/spf13/cobra v1.7.0
	github.com/spf13/viper v1.15.0
	github.com/starius/api2 v0.2.18
	github.com/starius/unifynil v0.0.0-20220116024101-fb5f67afeb8e
	github.com/stretchr/testify v1.8.4
	github.com/syndtr/goleveldb v1.0.0
	github.com/xtaci/smux v1.5.24
	gitlab.com/NebulousLabs/demotemutex v0.0.0-20151003192217-235395f71c40
	gitlab.com/NebulousLabs/encoding v0.0.0-20200604091946-456c3dc907fe
	gitlab.com/NebulousLabs/entropy-mnemonics v0.0.0-20181018051301-7532f67e3500
	gitlab.com/NebulousLabs/errors v0.0.0-20200929122200-06c536cf6975
	gitlab.com/NebulousLabs/fastrand v0.0.0-20181126182046-603482d69e40
	gitlab.com/NebulousLabs/go-upnp v0.0.0-20211002182029-11da932010b6
	gitlab.com/NebulousLabs/log v0.0.0-20210609172545-77f6775350e2
	gitlab.com/NebulousLabs/monitor v0.0.0-20191205095550-2b0fd3e1012a
	gitlab.com/NebulousLabs/ratelimit v0.0.0-20200811080431-99b8f0768b2e
	gitlab.com/NebulousLabs/siamux v0.0.1
	gitlab.com/NebulousLabs/threadgroup v0.0.0-20200608151952-38921fbef213
	gitlab.com/scpcorp/merkletree v0.0.0-20220107002940-1145778ea123
	gitlab.com/scpcorp/writeaheadlog v0.0.0-20200814111317-c404cb85e61f
	gitlab.com/zer0main/checkport v0.0.0-20211117123614-ea09614c7660
	gitlab.com/zer0main/eventsourcing v0.0.0-20210911223220-4432c7e50e57
	gitlab.com/zer0main/filestorage v0.0.0-20211220182308-d090285b251e
	go.etcd.io/bbolt v1.3.7
	golang.org/x/crypto v0.14.0
	golang.org/x/net v0.16.0
)

require github.com/mr-tron/base58 v1.2.0 // indirect

require (
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/getkin/kin-openapi v0.116.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/invopop/yaml v0.2.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.4 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/pelletier/go-toml/v2 v2.0.7 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/petermattis/goid v0.0.0-20230516130339-69c5d00fc54d // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spf13/afero v1.9.5 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/starius/flock v0.0.0-20211126131212-41983f66ca4f // indirect
	github.com/subosito/gotenv v1.4.2 // indirect
	gitlab.com/NebulousLabs/persist v0.0.0-20200605115618-007e5e23d877 // indirect
	gitlab.com/scpcorp/spf-transporter v0.0.0
	golang.org/x/mod v0.13.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/term v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/tools v0.14.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
