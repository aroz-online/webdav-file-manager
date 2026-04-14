#!/bin/bash
cd ../
make all

# Uncomment the one with suitable build platform
# ./build/webdav-file-manager_linux_amd64 -introspect > .introspect
./build/webdav-file-manager_windows_amd64.exe -introspect > .introspect