package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ngochc/dev-dash/internal/app"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stderr))
}

func run(ctx context.Context, args []string, errorOutput io.Writer) int {
	if err := app.Run(ctx, args); err != nil {
		fmt.Fprintln(errorOutput, "devdash:", err)
		return 1
	}
	return 0
}
