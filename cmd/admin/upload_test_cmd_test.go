package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
)

type fakeUploadPhotos struct {
	uploadErr, destroyErr error
	destroyed             []string
}

func (f *fakeUploadPhotos) Upload(_ context.Context, owner string, data []byte) (imagesdomain.Ref, error) {
	if f.uploadErr != nil {
		return imagesdomain.Ref{}, f.uploadErr
	}
	if owner != uploadTestOwner || len(data) == 0 {
		return imagesdomain.Ref{}, errors.New("datos inesperados")
	}
	return imagesdomain.Ref{PublicID: "vaiven/production/item_photo/upload-test/x"}, nil
}

func (f *fakeUploadPhotos) Destroy(_ context.Context, ref imagesdomain.Ref) error {
	f.destroyed = append(f.destroyed, ref.PublicID)
	return f.destroyErr
}

type fakeResolver struct {
	imgport.Service
	err error
}

func (r fakeResolver) Resolve(context.Context, string, imagesdomain.Purpose, string, []imagesdomain.Ref) ([]imagesdomain.Image, error) {
	return nil, r.err
}

func runFakeUploadTest(photos *fakeUploadPhotos, resolveErr error) (int, string) {
	var out bytes.Buffer
	cfg := config.Config{}
	cfg.Cloudinary.BaseFolder = "vaiven/production"
	code := runUploadTest(&out, cfg, func(config.Config) (uploadTestDeps, error) {
		return uploadTestDeps{images: fakeResolver{err: resolveErr}, photos: photos}, nil
	})
	return code, out.String()
}

func TestUploadTestUploadsVerifiesAndDeletes(t *testing.T) {
	p := &fakeUploadPhotos{}
	code, out := runFakeUploadTest(p, nil)
	if code != 0 || len(p.destroyed) != 1 || !strings.Contains(out, "OK       borrado") {
		t.Fatalf("código %d, borradas %v:\n%s", code, p.destroyed, out)
	}
}

func TestUploadTestDeletesEvenIfVerificationFails(t *testing.T) {
	p := &fakeUploadPhotos{}
	code, out := runFakeUploadTest(p, errors.New("no verificada"))
	if code != 1 || len(p.destroyed) != 1 {
		t.Fatalf("código %d, borradas %v:\n%s", code, p.destroyed, out)
	}
}

func TestUploadTestFailures(t *testing.T) {
	if code, _ := runFakeUploadTest(&fakeUploadPhotos{uploadErr: errors.New("x")}, nil); code != 1 {
		t.Errorf("subida fallida: código %d", code)
	}
	if code, _ := runFakeUploadTest(&fakeUploadPhotos{destroyErr: errors.New("x")}, nil); code != 1 {
		t.Errorf("borrado fallido: código %d", code)
	}
	var out bytes.Buffer
	code := runUploadTest(&out, config.Config{}, func(config.Config) (uploadTestDeps, error) {
		return uploadTestDeps{}, errors.New("sin credenciales")
	})
	if code != 1 {
		t.Errorf("sin configuración: código %d", code)
	}
}
