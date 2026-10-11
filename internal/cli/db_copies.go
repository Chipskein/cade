package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/chipskein/cade/internal/dbcopy"
)

var (
	databaseCopyNoun = nounForms{"cópia do banco", "cópias do banco", "database copy", "database copies"}
	removedCopyNoun  = nounForms{"cópia apagada", "cópias apagadas", "copy deleted", "copies deleted"}
)

// megabytesOf rounds up, so a small file never shows as 0 MB.
func megabytesOf(size int64) int64 {
	return (size + 1<<20 - 1) >> 20
}

func (env commandEnv) databaseCopies() ([]dbcopy.Copy, string, error) {
	cfg, err := env.loadConfig()
	if err != nil {
		return nil, "", err
	}
	copies, err := dbcopy.Find(env.toolkit.RootFS, cfg.DatabasePath)
	return copies, cfg.DatabasePath, err
}

// warnAboutCopies follows every forget: the copies taken before a
// migration or a rekey still hold what was just deleted (#87). Without
// copies it says nothing.
func (env commandEnv) warnAboutCopies() error {
	copies, _, err := env.databaseCopies()
	if err != nil || len(copies) == 0 {
		return err
	}
	fmt.Fprintf(env.stderr, env.language.pick("%s ainda guardam o que foi apagado:\n", "%s still hold what was deleted:\n"),
		env.language.count(len(copies), databaseCopyNoun))
	writeCopies(env.stderr, copies)
	fmt.Fprintln(env.stderr, env.language.pick("Apague com `cade forget --backups`.", "Delete them with `cade forget --backups`."))
	return nil
}

func writeCopies(out io.Writer, copies []dbcopy.Copy) {
	for _, copied := range copies {
		fmt.Fprintf(out, "  %s  (%s, %d MB)\n", copied.Path, copied.ModifiedAt.Format(time.DateOnly), megabytesOf(copied.Size))
	}
}

// forgetCopies lists the copies and deletes them once confirmed; the
// database itself is never one of them.
func (env commandEnv) forgetCopies(yes bool) error {
	copies, database, err := env.databaseCopies()
	if err != nil || len(copies) == 0 {
		if err == nil {
			fmt.Fprintln(env.stdout, env.language.pick("Nenhuma cópia do banco.", "No database copy."))
		}
		return err
	}
	writeCopies(env.stdout, copies)
	confirmed, err := env.confirm(yes, env.language.pick("Apagar estas cópias? [s/N] ", "Delete these copies? [y/N] "))
	if err != nil || !confirmed {
		return err
	}
	return env.removeCopies(copies, database)
}

func (env commandEnv) removeCopies(copies []dbcopy.Copy, database string) error {
	removed := 0
	for _, copied := range copies {
		if copied.Path == database {
			continue
		}
		if err := env.toolkit.RemoveFile(copied.Path); err != nil {
			return fmt.Errorf("delete the copy %q: %w", copied.Path, err)
		}
		removed++
	}
	fmt.Fprintln(env.stdout, env.language.count(removed, removedCopyNoun)+".")
	return nil
}
