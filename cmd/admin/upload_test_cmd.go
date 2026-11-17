package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
)

// upload-test (spec 013, T003): sube una foto con una firma REAL del entorno
// (carpeta CLOUDINARY_BASE_FOLDER), la verifica con Resolve como lo hace la API
// al publicar, y la borra. No escribe en MongoDB, así que se permite en
// producción (a diferencia de smoke).
const uploadTestTimeout = time.Minute

// uploadTestOwner es una carpeta de dueño ficticia: no corresponde a ninguna
// cuenta real.
const uploadTestOwner = "upload-test"

type uploadTestDeps struct {
	images imgport.Service
	photos photoStore
}

type uploadTestFactory func(cfg config.Config) (uploadTestDeps, error)

func newUploadTestDeps(cfg config.Config) (uploadTestDeps, error) {
	svc, err := images.New(images.Config{
		CloudName: cfg.Cloudinary.CloudName, APIKey: cfg.Cloudinary.APIKey,
		APISecret: cfg.Cloudinary.APISecret.Reveal(), BaseFolder: cfg.Cloudinary.BaseFolder,
		MaxBytes: cfg.Images.MaxBytes,
	})
	if err != nil {
		return uploadTestDeps{}, fmt.Errorf("configurar Cloudinary: %w", err)
	}
	return uploadTestDeps{images: svc, photos: newCloudinaryPhotos(svc, cfg.Cloudinary.CloudName, cfg.Cloudinary.APIKey, cfg.Cloudinary.APISecret.Reveal())}, nil
}

// runUploadTest devuelve 0 si la subida, la verificación y el borrado pasan.
func runUploadTest(stdout io.Writer, cfg config.Config, open uploadTestFactory) int {
	deps, err := open(cfg)
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "FALLA    configuración          %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), uploadTestTimeout)
	defer cancel()
	photo, err := seedAssets.ReadFile("assets/objeto-01.jpg")
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "FALLA    foto de prueba         %v\n", err)
		return 1
	}
	ref, err := deps.photos.Upload(ctx, uploadTestOwner, photo)
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "FALLA    subida firmada         %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "OK       subida firmada         carpeta %s\n", cfg.Cloudinary.BaseFolder)
	code := 0
	if _, err := deps.images.Resolve(ctx, uploadTestOwner, imagesdomain.PurposeItemPhoto, "photos", []imagesdomain.Ref{ref}); err != nil {
		_, _ = fmt.Fprintf(stdout, "FALLA    verificación           %v\n", err)
		code = 1
	} else {
		_, _ = fmt.Fprintln(stdout, "OK       verificación")
	}
	if err := deps.photos.Destroy(context.WithoutCancel(ctx), ref); err != nil {
		_, _ = fmt.Fprintf(stdout, "FALLA    borrado                %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "OK       borrado")
	return code
}
