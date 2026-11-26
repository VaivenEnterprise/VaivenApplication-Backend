# El token NUNCA va en archivos: se lee de BETTERUPTIME_API_TOKEN en el
# entorno de quien ejecuta terraform (o de un secreto de Actions).
provider "betteruptime" {}
