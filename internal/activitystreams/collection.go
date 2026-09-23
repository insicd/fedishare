package activitystreams

import "strconv"

// OrderedCollection is an ActivityStreams collection without items.
type OrderedCollection struct {
	Context    any    `json:"@context"`
	ID         string `json:"id"`
	Type       string `json:"type"`
	TotalItems int    `json:"totalItems"`
	First      string `json:"first,omitempty"`
	Last       string `json:"last,omitempty"`
}

// OrderedCollectionPage is one page of an OrderedCollection.
type OrderedCollectionPage struct {
	Context      any    `json:"@context"`
	ID           string `json:"id"`
	Type         string `json:"type"`
	PartOf       string `json:"partOf"`
	Next         string `json:"next,omitempty"`
	Prev         string `json:"prev,omitempty"`
	OrderedItems []any  `json:"orderedItems"`
}

const DefaultPageSize = 20

func Collection(id string, total int, pageSize int) OrderedCollection {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	col := OrderedCollection{
		Context:    Context,
		ID:         id,
		Type:       "OrderedCollection",
		TotalItems: total,
	}
	if total > 0 {
		pages := (total + pageSize - 1) / pageSize
		col.First = id + "?page=1"
		if pages > 1 {
			col.Last = id + "?page=" + strconv.Itoa(pages)
		} else {
			col.Last = col.First
		}
	}
	return col
}

func CollectionPage(id, partOf, next, prev string, items []any) OrderedCollectionPage {
	if items == nil {
		items = []any{}
	}
	return OrderedCollectionPage{
		Context:      Context,
		ID:           id,
		Type:         "OrderedCollectionPage",
		PartOf:       partOf,
		Next:         next,
		Prev:         prev,
		OrderedItems: items,
	}
}
