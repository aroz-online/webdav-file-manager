.PHONY: all

all: linux_386 linux_amd64 linux_arm linux_arm64 linux_mipsle linux_riscv64 windows_amd64.exe

linux_386:
	mkdir -p ./build
	CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -o webdav-file-manager_linux_386
	mv webdav-file-manager_linux_386 ./build/

linux_amd64:
	mkdir -p ./build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o webdav-file-manager_linux_amd64
	mv webdav-file-manager_linux_amd64 ./build/

linux_arm:
	mkdir -p ./build
	CGO_ENABLED=0 GOOS=linux GOARCH=arm go build -o webdav-file-manager_linux_arm
	mv webdav-file-manager_linux_arm ./build/

linux_arm64:
	mkdir -p ./build
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o webdav-file-manager_linux_arm64
	mv webdav-file-manager_linux_arm64 ./build/

linux_mipsle:
	mkdir -p ./build
	CGO_ENABLED=0 GOOS=linux GOARCH=mipsle go build -o webdav-file-manager_linux_mipsle
	mv webdav-file-manager_linux_mipsle ./build/

linux_riscv64:
	mkdir -p ./build
	CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -o webdav-file-manager_linux_riscv64
	mv webdav-file-manager_linux_riscv64 ./build/

windows_amd64.exe:
	mkdir -p ./build
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o webdav-file-manager_windows_amd64.exe
	mv webdav-file-manager_windows_amd64.exe ./build/

.PHONY: all linux_386 linux_amd64 linux_arm linux_arm64 linux_mipsle linux_riscv64 windows_amd64.exe