# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: fbox android ios fbox-cross swarm evm all test clean
.PHONY: fbox-linux fbox-linux-386 fbox-linux-amd64 fbox-linux-mips64 fbox-linux-mips64le
.PHONY: fbox-linux-arm fbox-linux-arm-5 fbox-linux-arm-6 fbox-linux-arm-7 fbox-linux-arm64
.PHONY: fbox-darwin fbox-darwin-386 fbox-darwin-amd64
.PHONY: fbox-windows fbox-windows-386 fbox-windows-amd64
.PHONY: docker

GOBIN = $(shell pwd)/build/bin
GOSHX = $(shell pwd)
GO ?= latest

fbox:
	build/env.sh go run build/ci.go install ./cmd/fbox
	@echo "Done building."
	@echo "Run \"$(GOBIN)/fbox\" to launch fbox."

promfile:
	build/env.sh go run build/ci.go install ./consensus/promfile
	@echo "Done building."
	@echo "Run \"$(GOBIN)/promfile\" to launch promfile."

all:
	build/env.sh go run build/ci.go install ./cmd/fbox
	@echo "Done building."
	@echo "Run \"$(GOBIN)/fbox\" to launch fbox."
	
	build/env.sh go run build/ci.go install ./consensus/promfile
	@echo "Done building."
	@echo "Run \"$(GOBIN)/promfile\" to launch promfile."

android:
	build/env.sh go run build/ci.go aar --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/fbox.aar\" to use the library."

ios:
	build/env.sh go run build/ci.go xcode --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/fbox.framework\" to use the library."

test: all
	build/env.sh go run build/ci.go test

clean:
	rm -fr build/workspace/pkg/ $(GOBIN)/*

# The devtools target installs tools required for 'go generate'.
# You need to put $GOBIN (or $GOPATH/bin) in your PATH to use 'go generate'.

devtools:
	env GOBIN= go get -u golang.org/x/tools/cmd/stringer
	env GOBIN= go get -u github.com/jteeuwen/go-bindata/go-bindata
	env GOBIN= go get -u github.com/fjl/gencodec
	env GOBIN= go install ./cmd/abigen

# Cross Compilation Targets (xgo)

fbox-cross: fbox-linux fbox-darwin fbox-windows fbox-android fbox-ios
	@echo "Full cross compilation done:"
	@ls -ld $(GOBIN)/fbox-*

fbox-linux: fbox-linux-386 fbox-linux-amd64 fbox-linux-arm fbox-linux-mips64 fbox-linux-mips64le
	@echo "Linux cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-*

fbox-linux-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/386 -v ./cmd/fbox
	@echo "Linux 386 cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-* | grep 386

fbox-linux-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/amd64 -v ./cmd/fbox
	@echo "Linux amd64 cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-* | grep amd64

fbox-linux-arm: fbox-linux-arm-5 fbox-linux-arm-6 fbox-linux-arm-7 fbox-linux-arm64
	@echo "Linux ARM cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-* | grep arm

fbox-linux-arm-5:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-5 -v ./cmd/fbox
	@echo "Linux ARMv5 cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-* | grep arm-5

fbox-linux-arm-6:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-6 -v ./cmd/fbox
	@echo "Linux ARMv6 cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-* | grep arm-6

fbox-linux-arm-7:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-7 -v ./cmd/fbox
	@echo "Linux ARMv7 cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-* | grep arm-7

fbox-linux-arm64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm64 -v ./cmd/fbox
	@echo "Linux ARM64 cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-* | grep arm64

fbox-linux-mips:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips --ldflags '-extldflags "-static"' -v ./cmd/fbox
	@echo "Linux MIPS cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-* | grep mips

fbox-linux-mipsle:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mipsle --ldflags '-extldflags "-static"' -v ./cmd/fbox
	@echo "Linux MIPSle cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-* | grep mipsle

fbox-linux-mips64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64 --ldflags '-extldflags "-static"' -v ./cmd/fbox
	@echo "Linux MIPS64 cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-* | grep mips64

fbox-linux-mips64le:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64le --ldflags '-extldflags "-static"' -v ./cmd/fbox
	@echo "Linux MIPS64le cross compilation done:"
	@ls -ld $(GOBIN)/fbox-linux-* | grep mips64le

fbox-darwin: fbox-darwin-386 fbox-darwin-amd64
	@echo "Darwin cross compilation done:"
	@ls -ld $(GOBIN)/fbox-darwin-*

fbox-darwin-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/386 -v ./cmd/fbox
	@echo "Darwin 386 cross compilation done:"
	@ls -ld $(GOBIN)/fbox-darwin-* | grep 386

fbox-darwin-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/amd64 -v ./cmd/fbox
	@echo "Darwin amd64 cross compilation done:"
	@ls -ld $(GOBIN)/fbox-darwin-* | grep amd64

fbox-windows: fbox-windows-386 fbox-windows-amd64
	@echo "Windows cross compilation done:"
	@ls -ld $(GOBIN)/fbox-windows-*

fbox-windows-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/386 -v ./cmd/fbox
	@echo "Windows 386 cross compilation done:"
	@ls -ld $(GOBIN)/fbox-windows-* | grep 386

fbox-windows-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/amd64 -v ./cmd/fbox
	@echo "Windows amd64 cross compilation done:"
	@ls -ld $(GOBIN)/fbox-windows-* | grep amd64

docker:
	docker build -t fastbox/fbox:latest .
