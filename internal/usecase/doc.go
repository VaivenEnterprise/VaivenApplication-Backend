// Package usecase contendrá los casos de uso de VAIVÉN y los puertos
// (interfaces) que ellos consumen. En la spec 002 no hay casos de uso: el
// paquete existe para fijar la capa (Art. III). Solo puede importar domain;
// nunca net/http, chi, mongo ni bson.
package usecase
