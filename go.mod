module mvw.org/cctools

go 1.22.0

toolchain go1.23.4

require (
	github.com/eiannone/keyboard v0.0.0-20200508000154-caf4b762e807
	github.com/gocarina/gocsv v0.0.0-20210516172204-ca9e8a8ddea8
	github.com/pkg/errors v0.9.1
	github.com/spf13/pflag v1.0.5
	gitlab.com/gomidi/midi v1.21.0
	gitlab.com/gomidi/rtmididrv v0.14.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	golang.org/x/sys v0.0.0-20210615035016-665e8c7367d1 // indirect
)

replace mvw.org/cctools => ./
