package project

import (
	"cmp"
	"slices"

	"github.com/igorhub/devcard"
)

type DevcardsMetaSlice []devcard.DevcardMeta

func (ds DevcardsMetaSlice) Lookup(name string) devcard.DevcardMeta {
	for _, meta := range ds {
		if meta.Name == name {
			return meta
		}
	}
	return devcard.DevcardMeta{}
}

func (ds DevcardsMetaSlice) FilterByImportPath(importPath string) DevcardsMetaSlice {
	metacards := slices.Clone(ds)
	metacards = slices.DeleteFunc(metacards, func(c devcard.DevcardMeta) bool {
		return c.ImportPath != importPath
	})
	return DevcardsMetaSlice(metacards)
}

func (ds DevcardsMetaSlice) GroupByImportPath() []DevcardsMetaSlice {
	cards := slices.Clone(ds)
	slices.SortStableFunc(cards, func(a, b devcard.DevcardMeta) int {
		return cmp.Compare(a.ImportPath, b.ImportPath)
	})

	ret := []DevcardsMetaSlice{}
	var importPath string
	for _, card := range cards {
		if card.ImportPath != importPath {
			ret = append(ret, []devcard.DevcardMeta{})
			importPath = card.ImportPath
		}
		ret[len(ret)-1] = append(ret[len(ret)-1], card)
	}

	return ret
}
