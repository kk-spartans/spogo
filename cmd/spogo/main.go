package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
	"github.com/steipete/spogo/internal/app"
	"github.com/steipete/spogo/internal/cli"
	"github.com/steipete/spogo/internal/output"
)

var exitFunc = os.Exit

func main() {
	exitFunc(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out io.Writer, errOut io.Writer) int {
	if isDaemonCommand(args) {
		return runDaemonCommand(args, out, errOut)
	}
	if code, ok := proxyToDaemon(args, out, errOut); ok {
		return code
	}
	return runLocal(args, out, errOut)
}

func runLocal(args []string, out io.Writer, errOut io.Writer) int {
	return runWithContext(args, out, errOut, app.NewContext, nil, false)
}

func runWithContext(
	args []string,
	out io.Writer,
	errOut io.Writer,
	buildContext func(app.Settings) (*app.Context, error),
	afterRun func(*app.Context),
	bindOutput bool,
) int {
	command := cli.New()
	exitCode := -1
	parser, err := kong.New(command,
		kong.Name("spogo"),
		kong.Description("Spotify power CLI using web cookies."),
		kong.UsageOnError(),
		kong.Writers(out, errOut),
		kong.Vars(cli.VersionVars()),
		kong.Exit(func(code int) {
			exitCode = code
		}),
	)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 2
	}
	kctx, err := parser.Parse(args)
	if exitCode >= 0 {
		return exitCode
	}
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 2
	}
	settings, err := command.Globals.Settings()
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 2
	}
	ctx, err := buildContext(settings)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	if bindOutput {
		writer, writerErr := output.New(output.Options{
			Format: settings.Format,
			Color:  false,
			Out:    out,
			Err:    errOut,
			Quiet:  settings.Quiet,
		})
		if writerErr != nil {
			_, _ = fmt.Fprintln(errOut, writerErr)
			return 1
		}
		ctx.Output = writer
	}
	if afterRun != nil {
		defer afterRun(ctx)
	}
	ctx.SetCommandContext(context.Background())
	if err := ctx.ValidateProfile(); err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 2
	}
	if err := kctx.Run(ctx); err != nil {
		ctx.Output.Errorf("%v", err)
		return app.ExitCode(err)
	}
	return 0
}
