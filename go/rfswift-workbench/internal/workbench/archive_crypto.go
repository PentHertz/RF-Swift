package workbench

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"

	"filippo.io/age"
)

const ageHeader = "age-encryption.org/v1"

func archiveIsEncrypted(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	b := make([]byte, len(ageHeader))
	n, err := io.ReadFull(f, b)
	if err != nil && err != io.ErrUnexpectedEOF {
		return false, err
	}
	return string(b[:n]) == ageHeader, nil
}

func encryptArchive(source, destination, password string) (err error) {
	if strings.TrimSpace(password) == "" {
		return errors.New("archive password is empty")
	}
	recipient, err := age.NewScryptRecipient(password)
	if err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(destination)
		}
	}()
	w, err := age.Encrypt(out, recipient)
	if err != nil {
		return err
	}
	if _, err = io.Copy(w, in); err != nil {
		return err
	}
	return w.Close()
}

func decryptArchive(source, destination, password string) (err error) {
	if strings.TrimSpace(password) == "" {
		return errors.New("this archive is password-protected; enter its password")
	}
	identity, err := age.NewScryptIdentity(password)
	if err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	r, err := age.Decrypt(bufio.NewReader(in), identity)
	if err != nil {
		return errors.New("archive password is incorrect or the encrypted archive is damaged")
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(destination)
		}
	}()
	_, err = io.Copy(out, r)
	return err
}
