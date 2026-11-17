package config

// Cloudinary es la configuración del servicio de imágenes (RF-S2, P-S2).
// El secreto vive solo en el backend (Art. VI.7).
type Cloudinary struct {
	CloudName  string
	APIKey     string
	APISecret  Secret
	BaseFolder string
}

// loadCloudinary exige las credenciales en todos los entornos salvo test y
// usa vaiven/<APP_ENV> como carpeta base por defecto.
func loadCloudinary(r *reader, appEnv string) Cloudinary {
	c := Cloudinary{BaseFolder: r.text("CLOUDINARY_BASE_FOLDER", "vaiven/"+appEnv)}
	if appEnv == EnvTest {
		c.CloudName = r.text("CLOUDINARY_CLOUD_NAME", "")
		c.APIKey = r.text("CLOUDINARY_API_KEY", "")
		c.APISecret = Secret(r.text("CLOUDINARY_API_SECRET", ""))
		return c
	}
	c.CloudName = r.required("CLOUDINARY_CLOUD_NAME")
	c.APIKey = r.required("CLOUDINARY_API_KEY")
	c.APISecret = Secret(r.required("CLOUDINARY_API_SECRET"))
	return c
}
