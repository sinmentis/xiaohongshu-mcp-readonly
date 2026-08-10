package browser

import (
	"archive/tar"
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"
)

func TestExpectedSHA(t *testing.T) {
	hash, err := expectedSHA("linux-x64.tar.xz")

	require.NoError(t, err)
	assert.Len(t, hash, 64)
	assert.EqualError(
		t,
		func() error {
			_, err := expectedSHA("unknown.zip")
			return err
		}(),
		"browser_sha256s.txt does not contain unknown.zip",
	)
}

func TestArchiveTargetRejectsTraversal(t *testing.T) {
	root := t.TempDir()

	target, err := archiveTarget(root, "chrome/chrome")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "chrome", "chrome"), target)

	for _, name := range []string{"../escape", "chrome/../../escape", "/tmp/escape"} {
		t.Run(name, func(t *testing.T) {
			_, err := archiveTarget(root, name)
			assert.Error(t, err)
		})
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "browser.zip")
	file, err := os.Create(archivePath)
	require.NoError(t, err)
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escape")
	require.NoError(t, err)
	_, err = entry.Write([]byte("blocked"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())

	err = extractZip(archivePath, root)

	assert.Error(t, err)
	_, statErr := os.Stat(filepath.Join(filepath.Dir(root), "escape"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestExtractTarXzRejectsLinks(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "browser.tar.xz")
	file, err := os.Create(archivePath)
	require.NoError(t, err)
	xzWriter, err := xz.NewWriter(file)
	require.NoError(t, err)
	tarWriter := tar.NewWriter(xzWriter)
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{
		Name:     "chrome-link",
		Typeflag: tar.TypeSymlink,
		Linkname: "../../escape",
	}))
	require.NoError(t, tarWriter.Close())
	require.NoError(t, xzWriter.Close())
	require.NoError(t, file.Close())

	err = extractTarXz(archivePath, t.TempDir())

	assert.EqualError(t, err, "browser archive contains a disallowed link: chrome-link")
}
