package main

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func privateDefaults() {}
func privateConsole() (*os.File, error) {
	f, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
func secureStore(dir string) error {
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return errors.New("cannot identify current Windows user")
	}
	defer tok.Close()
	user, err := tok.GetTokenUser()
	if err != nil {
		return errors.New("cannot identify current Windows user")
	}
	sid := user.User.Sid
	s := sid.String()
	sd, err := windows.SecurityDescriptorFromString("O:" + s + "D:P(A;OICI;FA;;;" + s + ")")
	if err != nil {
		return errors.New("cannot prepare owner-only Windows ACL")
	}
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	attr := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if err := windows.CreateDirectory(path, &attr); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return errors.New("cannot create owner-only Windows directory")
	}
	if err := verifyWindowsACL(dir, sid, true); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return errors.New("cannot inspect monitor directory")
	}
	for _, e := range entries {
		switch e.Name() {
		case "session.db", "session.db-journal", "session.db-wal", "session.db-shm", "messages.db", "messages.db-journal", "messages.db-wal", "messages.db-shm":
			if err := verifyWindowsACL(filepath.Join(dir, e.Name()), sid, false); err != nil {
				return err
			}
		default:
			return errors.New("monitor directory has unexpected contents; use a dedicated directory")
		}
	}
	return nil
}
func verifyWindowsACL(name string, sid *windows.SID, directory bool) error {
	bad := errors.New("monitor store must use an owner-only ACL on real Windows files and directory")
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return bad
	}
	fa, err := windows.GetFileAttributes(path)
	if err != nil || fa&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (fa&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		return bad
	}
	sd, err := windows.GetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return bad
	}
	owner, _, err := sd.Owner()
	if err != nil || !sid.Equals(owner) {
		return bad
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount != 1 {
		return bad
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(acl, 0, &ace); err != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !sid.Equals((*windows.SID)(unsafe.Pointer(&ace.SidStart))) {
		return bad
	}
	if directory {
		const objectAndContainerInherit = 3
		control, _, err := sd.Control()
		if err != nil || control&windows.SE_DACL_PROTECTED == 0 || ace.Header.AceFlags&objectAndContainerInherit != objectAndContainerInherit {
			return bad
		}
	}
	return nil
}
