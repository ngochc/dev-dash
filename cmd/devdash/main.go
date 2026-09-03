package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ngochc/dev-dash/internal/app"
)

func main() {
	if err := app.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "devdash:", err)
		os.Exit(1)
	}
}
