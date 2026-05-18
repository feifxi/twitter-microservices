module github.com/twitter/dlq-replay

go 1.26

require (
	github.com/segmentio/kafka-go v0.4.51
	github.com/twitter/shared v0.0.0-00010101000000-000000000000
)

require (
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/pierrec/lz4/v4 v4.1.16 // indirect
)

replace github.com/twitter/shared => ../shared
