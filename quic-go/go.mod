module github.com/lucas-clemente/quic-go

go 1.20

require (
	// github.com/bifurcation/mint v0.0.0-20180715133206-93c51c6ce115
	github.com/golang/mock v1.2.0
	github.com/hashicorp/golang-lru v0.5.1
	github.com/lucas-clemente/aes12 v0.0.0-20171027163421-cd47fb39b79f
	github.com/lucas-clemente/fnv128a v0.0.0-20160504152609-393af48d3916
	github.com/lucas-clemente/quic-clients v0.1.0
	github.com/lucas-clemente/quic-go-certificates v0.0.0-20160823095156-d2f86524cced
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	golang.org/x/crypto v0.19.0
	golang.org/x/net v0.21.0
)

require (
	github.com/bifurcation/mint v0.0.0-20171208133358-a6080d464fb5
	github.com/gabriel-vasile/mimetype v1.4.3
)

require (
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/hpcloud/tail v1.0.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	gonum.org/v1/gonum v0.0.0-20180816165407-929014505bf4
	google.golang.org/protobuf v1.32.0 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.2.3 // indirect
)

replace gonum.org/v1/gonum => ../../../gonum.org/v1/gonum

// replace github.com/bifurcation/mint => ../../../github.com/bifurcation/mint
