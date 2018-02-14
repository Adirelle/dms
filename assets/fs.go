package assets

import (
	assetfs "github.com/elazarl/go-bindata-assetfs"
	bindata "github.com/jteeuwen/go-bindata"
)

//go:generate go-bindata -o assets.go       -pkg assets -tags !debug -nocompress -prefix files/ files/...
//go:generate go-bindata -o assets_debug.go -pkg assets -tags debug  -debug      -prefix files/ files/...

var FileSystem = &assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, AssetInfo: AssetInfo}

// unexported, unused var to force import of github.com/jteeuwen/go-bindata
var dummy = bindata.Asset{}
