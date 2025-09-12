#!/bin/bash

echo "Building Pooshit..."
go build -o pooshit

if [ $? -eq 0 ]; then
    echo "Build successful! Run with: ./pooshit"
    chmod +x pooshit
else
    echo "Build failed!"
    exit 1
fi
