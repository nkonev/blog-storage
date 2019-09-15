package migrate_rice

import (
	"bytes"
	"fmt"
	rice "github.com/GeertJohan/go.rice"
	"github.com/golang-migrate/migrate/v4/source"
	"io"
	"io/ioutil"
	"os"
)

const Name = "rice"

func init() {
	source.Register(Name, &Packr{})
}

type Packr struct {
	migrations *source.Migrations
	box        *rice.Box
	path       string
}

func (b *Packr) Open(url string) (source.Driver, error) {
	return nil, fmt.Errorf("not yet implemented, please use WithInstance")
}

func WithInstance(instance interface{}) (source.Driver, error) {
	if _, ok := instance.(*rice.Box); !ok {
		return nil, fmt.Errorf("expects *packr.Box")
	}
	bx := instance.(*rice.Box)

	driver := &Packr{
		box:        bx,
		migrations: source.NewMigrations(),
		path:       bx.Name(),
	}

	if err := bx.Walk("", func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		m, err := source.DefaultParse(path)
		if err != nil {
			return err
		}

		if !driver.migrations.Append(m) {
			return fmt.Errorf("unable to parse file %v", path)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return driver, nil
}

func (b *Packr) Close() error {
	return nil
}

func (b *Packr) First() (version uint, err error) {
	if v, ok := b.migrations.First(); !ok {
		return 0, &os.PathError{"first", b.path, os.ErrNotExist}
	} else {
		return v, nil
	}
}

func (b *Packr) Prev(version uint) (prevVersion uint, err error) {
	if v, ok := b.migrations.Prev(version); !ok {
		return 0, &os.PathError{fmt.Sprintf("prev for version %v", version), b.path, os.ErrNotExist}
	} else {
		return v, nil
	}
}

func (b *Packr) Next(version uint) (nextVersion uint, err error) {
	if v, ok := b.migrations.Next(version); !ok {
		return 0, &os.PathError{fmt.Sprintf("next for version %v", version), b.path, os.ErrNotExist}
	} else {
		return v, nil
	}
}

func (b *Packr) ReadUp(version uint) (r io.ReadCloser, identifier string, err error) {
	if m, ok := b.migrations.Up(version); ok {
		body := b.box.MustBytes(m.Raw)
		return ioutil.NopCloser(bytes.NewReader(body)), m.Identifier, nil
	}
	return nil, "", &os.PathError{fmt.Sprintf("read version %v", version), b.path, os.ErrNotExist}
}

func (b *Packr) ReadDown(version uint) (r io.ReadCloser, identifier string, err error) {
	if m, ok := b.migrations.Down(version); ok {
		body := b.box.MustBytes(m.Raw)
		return ioutil.NopCloser(bytes.NewReader(body)), m.Identifier, nil
	}
	return nil, "", &os.PathError{fmt.Sprintf("read version %v", version), b.path, os.ErrNotExist}
}