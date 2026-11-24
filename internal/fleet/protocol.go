package fleet

const AgentLeaseName = "agent"

// AllowedRolesAnnotation, on the agent's Lease, lists the roles it may bind: sorted names joined
// by commas, "*" for any role. The agent omits it when it cannot tell.
const AllowedRolesAnnotation = "protem.dev/allowed-roles"

// AllowedRolesPartialAnnotation is "true" beside AllowedRolesAnnotation when the cluster's
// authorizer could not list every rule, so the list is a lower bound.
const AllowedRolesPartialAnnotation = "protem.dev/allowed-roles-partial"

func ClusterNamespace(cluster string) string {
	return "protem-cluster-" + cluster
}

func GrantKey(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "." + name
}
