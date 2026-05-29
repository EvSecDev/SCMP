#!/bin/bash
# All build tasks are done through the go builder program
go run cmd/builder/main.go "$@"
