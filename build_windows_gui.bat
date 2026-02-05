@echo off
setlocal
go build -ldflags "-H=windowsgui" -o ip-opt-gui.exe .
