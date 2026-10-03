package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type botGamesSocketListener struct {
	*net.UnixListener
	directory *os.File
	path      string
	inode     os.FileInfo
	once      sync.Once
	closeErr  error
}

func botGamesSocketOwned(info os.FileInfo) bool {
	owner, ok := info.Sys().(*syscall.Stat_t)
	return ok && owner.Uid == uint32(os.Geteuid())
}

// The directory is B-only and private. Holding its flock through Close serializes
// independent processes without leaving a lock file to survive SIGKILL/backup.
// A crashed process releases the lock but can leave the filesystem socket inode.
// Recovery only removes an owned socket after an explicit ECONNREFUSED and a
// same-inode recheck; ambiguous failures never authorize unlinking an endpoint.

func openBotGamesListener(path string) (net.Listener, error) {
	if !filepath.IsAbs(path) {
		return nil, errBotGamesListener
	}
	dir, err := os.OpenFile(filepath.Dir(path), os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errBotGamesListener
	}
	keep := false
	defer func() {
		if !keep {
			_ = dir.Close()
		}
	}()
	directory, err := dir.Stat()
	if err != nil || !directory.IsDir() || !botGamesSocketOwned(directory) || directory.Mode().Perm() != 0700 {
		return nil, errBotGamesListener
	}
	if err := syscall.Flock(int(dir.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, errBotGamesListener
	}
	currentDir, err := os.Lstat(filepath.Dir(path))
	if err != nil || !os.SameFile(directory, currentDir) {
		return nil, errBotGamesListener
	}
	if old, err := os.Lstat(path); err == nil {
		if old.Mode()&os.ModeSocket == 0 || !botGamesSocketOwned(old) {
			return nil, errBotGamesListener
		}
		conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil, errBotGamesListener
		}
		if !errors.Is(err, syscall.ECONNREFUSED) || removeBotGamesSocket(path, old) != nil {
			return nil, errBotGamesListener
		}
	} else if !os.IsNotExist(err) {
		return nil, errBotGamesListener
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, errBotGamesListener
	}
	listener.SetUnlinkOnClose(false)
	inode, err := os.Lstat(path)
	if err != nil || inode.Mode()&os.ModeSocket == 0 || !botGamesSocketOwned(inode) {
		_ = listener.Close()
		return nil, errBotGamesListener
	}
	if err := os.Chmod(path, 0600); err != nil {
		_ = listener.Close()
		_ = removeBotGamesSocket(path, inode)
		return nil, errBotGamesListener
	}
	keep = true
	return &botGamesSocketListener{UnixListener: listener, directory: dir, path: path, inode: inode}, nil
}

func removeBotGamesSocket(path string, inode os.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil || current.Mode()&os.ModeSocket == 0 || !botGamesSocketOwned(current) || !os.SameFile(inode, current) {
		return errBotGamesListener
	}
	return os.Remove(path)
}

func (l *botGamesSocketListener) Close() error {
	l.once.Do(func() {
		l.closeErr = l.UnixListener.Close()
		// A replaced/missing path is not ours to remove, including a later live
		// endpoint. The same inode guard also protects the stale recovery path.
		_ = removeBotGamesSocket(l.path, l.inode)
		_ = l.directory.Close()
	})
	return l.closeErr
}
