module github.com/filebrowser/filebrowser/v3

go 1.14

require (
	github.com/DataDog/zstd v1.4.5 // indirect
	github.com/Sereal/Sereal v0.0.0-20200611165018-70572ef94023 // indirect
	github.com/asdine/storm v2.1.2+incompatible
	github.com/didip/tollbooth/v6 v6.0.1
	github.com/didip/tollbooth_chi v0.0.0-20200524181329-8b84cd7183d9
	github.com/go-chi/chi v4.1.2+incompatible
	github.com/go-chi/cors v1.1.1
	github.com/go-chi/render v1.0.1
	github.com/go-pkgz/auth v0.11.0
	github.com/go-pkgz/lcw v0.7.1
	github.com/go-pkgz/rest v1.5.0
	github.com/golang/mock v1.4.3
	github.com/golang/protobuf v1.4.2 // indirect
	github.com/markbates/pkger v0.17.0
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.6.1
	github.com/umputun/go-flags v1.5.1
	github.com/vmihailenco/msgpack v4.0.4+incompatible // indirect
	go.etcd.io/bbolt v1.3.4
	go.uber.org/zap v1.15.0
	golang.org/x/crypto v0.0.0-20190530122614-20be4c3c3ed5
	google.golang.org/appengine v1.6.6 // indirect
)

replace github.com/go-pkgz/auth v0.11.0 => github.com/o1egl/auth v0.11.1-0.20200627171302-16caebf3ffdd
