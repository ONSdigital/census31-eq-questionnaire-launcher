#!/bin/bash
go list ./... | grep -v '/vendor/' | while IFS= read -r pkg; do
    go fmt "$pkg"
done
