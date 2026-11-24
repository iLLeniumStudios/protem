package api

import (
	"cmp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The informer cache lists objects in no particular order and creation timestamps have one-second
// resolution, so both orders end on namespace and name: two objects never compare equal.

func byName(a, b metav1.Object) int {
	return cmp.Or(cmp.Compare(a.GetNamespace(), b.GetNamespace()), cmp.Compare(a.GetName(), b.GetName()))
}

func newestFirst(a, b metav1.Object) int {
	return cmp.Or(
		b.GetCreationTimestamp().Compare(a.GetCreationTimestamp().Time),
		cmp.Compare(a.GetNamespace(), b.GetNamespace()),
		cmp.Compare(b.GetName(), a.GetName()),
	)
}
