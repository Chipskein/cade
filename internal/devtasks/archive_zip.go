package devtasks

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
)

// zipSuffix names the Windows release archive (#54).
const zipSuffix = ".zip"

// writeZip archives baseDir/entry with paths starting at entry, as
// writeTarGz does, for Windows, whose Explorer opens a zip.
func writeZip(archive, baseDir, entry string) error {
	file, err := os.Create(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	zipped := zip.NewWriter(file)
	err = filepath.WalkDir(filepath.Join(baseDir, entry), func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return addToZip(zipped, baseDir, path)
	})
	if err != nil {
		return err
	}
	if err := zipped.Close(); err != nil {
		return err
	}
	return file.Close()
}

// addToZip adds path under its name relative to baseDir, written with "/"
// as zip requires; a directory's name ends in one.
func addToZip(zipped *zip.Writer, baseDir, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(baseDir, path)
	if err != nil {
		return err
	}
	header.Name, header.Method = filepath.ToSlash(relative), zip.Deflate
	if info.IsDir() {
		header.Name += "/"
		_, err = zipped.CreateHeader(header)
		return err
	}
	destination, err := zipped.CreateHeader(header)
	if err != nil {
		return err
	}
	return copyInto(destination, path)
}
