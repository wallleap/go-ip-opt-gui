$ErrorActionPreference = "Stop"

go build -ldflags "-H=windowsgui" -o ip-opt-gui.exe .
