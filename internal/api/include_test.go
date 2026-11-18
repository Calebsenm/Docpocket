package api

import (
	"net/http"
	"testing"
)

func TestIncludeJoin(t *testing.T) {
	api := newTestAPI(t)
	api.setupAdmin()
	api.createCollection(publicBaseCollection("posts"))
	api.createCollection(publicBaseCollection("author"))
	api.createCollection(publicBaseCollection("project"))

	// Crear el autor y el proyecto con IDs conocidos para las referencias.
	// El nombre de la colección coincide con el campo de referencia del post,
	// tal como exige el formato "reference.field" del operador ?include=.
	author := api.json("POST", "/api/collections/author/records", map[string]any{
		"name": "Ana García",
	}, "", http.StatusCreated)
	authorID, _ := author["id"].(string)
	if authorID == "" {
		t.Fatalf("missing author id: %v", author)
	}

	project := api.json("POST", "/api/collections/project/records", map[string]any{
		"title": "DocPocket Blog",
	}, "", http.StatusCreated)
	projectID, _ := project["id"].(string)
	if projectID == "" {
		t.Fatalf("missing project id: %v", project)
	}

	// Crear un post que referencia al autor y al proyecto por sus IDs.
	api.json("POST", "/api/collections/posts/records", map[string]any{
		"title":   "Hola mundo",
		"author":  authorID,
		"project": projectID,
	}, "", http.StatusCreated)

	// Listar con include: los campos de otras colecciones deben resolverse.
	list := api.json("GET", "/api/collections/posts/records?include=author.name&include=project.title", nil, "", http.StatusOK)
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item: %v", list)
	}
	item, _ := items[0].(map[string]any)
	if item["author.name"] != "Ana García" {
		t.Fatalf("expected author.name = Ana García, got %v (item: %v)", item["author.name"], item)
	}
	if item["project.title"] != "DocPocket Blog" {
		t.Fatalf("expected project.title = DocPocket Blog, got %v (item: %v)", item["project.title"], item)
	}
	// El campo de referencia original debe permanecer intacto.
	if item["author"] != authorID {
		t.Fatalf("expected author reference to remain %v, got %v", authorID, item["author"])
	}
}

func TestIncludeJoinMissingReference(t *testing.T) {
	api := newTestAPI(t)
	api.setupAdmin()
	api.createCollection(publicBaseCollection("posts"))
	api.createCollection(publicBaseCollection("author"))

	// Post con una referencia que no existe en la colección author.
	api.json("POST", "/api/collections/posts/records", map[string]any{
		"title":  "Sin autor",
		"author": "01JX0000000000000000000000",
	}, "", http.StatusCreated)

	list := api.json("GET", "/api/collections/posts/records?include=author.name", nil, "", http.StatusOK)
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item: %v", list)
	}
	item, _ := items[0].(map[string]any)
	if v, ok := item["author.name"]; !ok || v != nil {
		t.Fatalf("expected author.name = null for missing reference, got %v (present=%v, item: %v)", v, ok, item)
	}
}
