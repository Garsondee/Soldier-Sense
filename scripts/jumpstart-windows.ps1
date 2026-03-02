# Use this script to install the tools needed to build and run Soldier Sense on Windows. 
winget install --id=Casey.Just -e
winget install --id GoLang.Go -e

go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
