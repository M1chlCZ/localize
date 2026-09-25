package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/m1chlcz/localize/arb"
	"github.com/m1chlcz/localize/store"
	"github.com/m1chlcz/localize/workbook"
)

const (
	maxWorkbookBytes int64 = 16 << 20
	fileMode               = 0o644
)

const usageMessage = "usage: localize export|validate|generate|import [options]"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := execute(ctx, os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execute(ctx context.Context, args []string, output io.Writer) error {
	if len(args) == 0 {
		return errors.New(usageMessage)
	}
	switch args[0] {
	case "export":
		return exportCommand(ctx, args[1:], output)
	case "validate":
		return validateCommand(args[1:], output)
	case "generate":
		return generateCommand(args[1:], output)
	case "import":
		return importCommand(ctx, args[1:], output)
	default:
		return errors.New(usageMessage)
	}
}

func exportCommand(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("localize export", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	storePath := flags.String("store", "", "path to the JSON store")
	path := flags.String("output", "", "output XLSX path")
	force := flags.Bool("force", false, "replace an existing output file")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *storePath == "" || *path == "" {
		return errors.New("usage: localize export --store FILE.json --output FILE.xlsx [--force]")
	}
	snapshot, err := store.JSONStore{Path: *storePath}.Snapshot(ctx)
	if err != nil {
		return err
	}
	snapshot.ExportedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeWorkbook(*path, snapshot, *force); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "exported revision %d to %s\n", snapshot.Revision, *path)
	return nil
}

func validateCommand(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("localize validate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("input", "", "input XLSX path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *path == "" {
		return errors.New("usage: localize validate --input FILE.xlsx")
	}
	item, err := readWorkbook(*path)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "workbook revision %d is valid\n", item.Revision)
	return nil
}

func generateCommand(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("localize generate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("input", "", "input XLSX path")
	outputDirectory := flags.String("output-dir", "", "generated artifact directory")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *path == "" || *outputDirectory == "" {
		return errors.New("usage: localize generate --input FILE.xlsx --output-dir DIRECTORY")
	}
	item, err := readWorkbook(*path)
	if err != nil {
		return err
	}
	artifacts, err := arb.Generate(item)
	if err != nil {
		return err
	}
	if err := writeArtifacts(*outputDirectory, artifacts); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "generated %d localization artifacts in %s\n", len(artifacts), *outputDirectory)
	return nil
}

func importCommand(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("localize import", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	storePath := flags.String("store", "", "path to the JSON store")
	path := flags.String("input", "", "input XLSX path")
	dryRun := flags.Bool("dry-run", false, "validate against the store and stop")
	apply := flags.Bool("apply", false, "apply the workbook to the store")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *storePath == "" || *path == "" {
		return errors.New("usage: localize import --store FILE.json --input FILE.xlsx (--dry-run|--apply)")
	}
	if *dryRun == *apply {
		return errors.New("import requires exactly one of --dry-run or --apply")
	}
	item, err := readWorkbook(*path)
	if err != nil {
		return err
	}
	result, err := store.JSONStore{Path: *storePath}.Apply(ctx, item, *dryRun)
	if err != nil {
		return err
	}
	mode := "dry-run"
	if result.Applied {
		mode = "applied"
	}
	_, _ = fmt.Fprintf(output, "%s %d changes; store revision %d\n", mode, result.Changes, result.Revision)
	return nil
}

func readWorkbook(path string) (workbook.Workbook, error) {
	info, err := os.Stat(path)
	if err != nil {
		return workbook.Workbook{}, fmt.Errorf("inspect workbook: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxWorkbookBytes {
		return workbook.Workbook{}, fmt.Errorf(
			"workbook must be a regular file between 1 and %d bytes",
			maxWorkbookBytes,
		)
	}
	file, err := os.Open(path)
	if err != nil {
		return workbook.Workbook{}, fmt.Errorf("open workbook: %w", err)
	}
	defer file.Close()
	item, err := workbook.Decode(io.LimitReader(file, maxWorkbookBytes+1))
	if err != nil {
		return workbook.Workbook{}, err
	}
	return item, nil
}

func writeWorkbook(path string, item workbook.Workbook, force bool) error {
	directory := filepath.Dir(path)
	if !force {
		if _, err := os.Stat(path); err == nil {
			return errors.New("output workbook already exists; use --force to replace it")
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect output workbook: %w", err)
		}
	}
	temporary, err := os.CreateTemp(directory, ".localization-*.xlsx")
	if err != nil {
		return fmt.Errorf("create temporary workbook: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := workbook.Encode(temporary, item); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync workbook: %w", err)
	}
	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set workbook permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close workbook: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish workbook: %w", err)
	}
	return nil
}

func writeArtifacts(directory string, artifacts []arb.Artifact) error {
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf("create generated directory: %w", err)
	}
	for _, artifact := range artifacts {
		if artifact.Path == "" || filepath.Base(artifact.Path) != artifact.Path {
			return fmt.Errorf("generated artifact has unsafe path %q", artifact.Path)
		}
		temporary, err := os.CreateTemp(directory, ".localization-*")
		if err != nil {
			return fmt.Errorf("create temporary artifact: %w", err)
		}
		temporaryPath := temporary.Name()
		published := false
		defer func() {
			if !published {
				_ = os.Remove(temporaryPath)
			}
		}()
		if _, err := temporary.Write(artifact.Content); err != nil {
			_ = temporary.Close()
			return fmt.Errorf("write generated artifact %s: %w", artifact.Path, err)
		}
		if err := temporary.Sync(); err != nil {
			_ = temporary.Close()
			return fmt.Errorf("sync generated artifact %s: %w", artifact.Path, err)
		}
		if err := temporary.Chmod(fileMode); err != nil {
			_ = temporary.Close()
			return fmt.Errorf("set generated artifact permissions: %w", err)
		}
		if err := temporary.Close(); err != nil {
			return fmt.Errorf("close generated artifact %s: %w", artifact.Path, err)
		}
		if err := os.Rename(temporaryPath, filepath.Join(directory, artifact.Path)); err != nil {
			return fmt.Errorf("publish generated artifact %s: %w", artifact.Path, err)
		}
		published = true
	}
	return nil
}
