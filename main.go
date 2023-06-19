package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/go-git/go-git/v5"
	"github.com/jessevdk/go-flags"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// Version is the version of the compiled software.
	Version = "dev"
	id, _   = os.Hostname()
)

var opts struct {
	Env      string `long:"env" env:"ENV" description:"Environment name" default:"development"`
	LogLevel string `long:"log-level" env:"LOG_LEVEL" description:"Log level" default:"info"`
	RepoURL  string `long:"repo-url" env:"REPO_URL" description:"Repo URL" default:"https://github.com/ravilushqa/boilerplate"`
	Project  string `long:"project" env:"PROJECT" description:"Project name" default:"new-project"`
	Dir      string `long:"dir" env:"DIR" description:"Directory" default:"./"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	_, err := flags.Parse(&opts)
	if err != nil {
		if err.(*flags.Error).Type != flags.ErrHelp {
			panic(err)
		}
		return
	}

	l := initLogger()

	if err := run(ctx, l); err != nil {
		l.Fatal("run failed", zap.Error(err))
	}
	fmt.Println("Done!")
	_ = l.Sync()
}

func run(ctx context.Context, l *zap.Logger) error {
	dir := opts.Dir + opts.Project

	// Clone the repo
	fmt.Println("Cloning the repo...")
	_, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		URL:      opts.RepoURL,
		Progress: os.Stdout,
	})
	if err != nil {
		return fmt.Errorf("clone failed: %w", err)
	}

	oldImport := strings.Replace(opts.RepoURL, "https://", "", 1)
	newImport := strings.Replace(oldImport, "boilerplate", opts.Project, 1)

	// Replace imports
	l.Info("Replacing imports...")
	if err = replaceImportsInDir(dir, oldImport, newImport); err != nil {
		return fmt.Errorf("replace imports failed: %w", err)
	}

	// remove .git folder and init new git repo
	l.Info("Removing .git folder...")
	if err = os.RemoveAll(dir + "/.git"); err != nil {
		return fmt.Errorf("remove .git failed: %w", err)
	}

	l.Info("Initializing new git repo...")
	if err = exec.CommandContext(ctx, "git", "init").Run(); err != nil {
		return fmt.Errorf("git init failed: %w", err)
	}

	// Run go mod tidy in the new project
	l.Info("Running go mod tidy...")
	if err = os.Chdir(dir); err != nil {
		return fmt.Errorf("chdir failed: %w", err)
	}

	err = exec.CommandContext(ctx, "go", "mod", "tidy").Run()
	if err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}

	return nil
}

func replaceInFile(filePath string, oldString string, newString string) error {
	read, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	newContents := strings.Replace(string(read), oldString, newString, -1)

	return os.WriteFile(filePath, []byte(newContents), 0)
}

func replaceImportsInDir(dir string, oldImport string, newImport string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), ".mod")) {
			if err = replaceInFile(path, oldImport, newImport); err != nil {
				return err
			}
		}

		return nil
	})
}

func initLogger() *zap.Logger {
	lcfg := zap.NewProductionConfig()
	atom := zap.NewAtomicLevel()
	_ = atom.UnmarshalText([]byte(opts.LogLevel))
	lcfg.Level = atom

	if opts.Env == "development" || opts.Env == "test" {
		lcfg = zap.NewDevelopmentConfig()
		lcfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	l, err := lcfg.Build(zap.Hooks())
	if err != nil {
		panic(fmt.Sprintf("failed to create logger: %v", err))
	}
	if err != nil {
		panic(fmt.Errorf("failed to init logger: %w", err))
	}
	l = l.With(zap.String("id", id), zap.String("version", Version))
	return l
}
