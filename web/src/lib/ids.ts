// Node ID format shared with the backend: apiVersion|kind|namespace|name.
export const makeNodeId = (apiVersion: string, kind: string, namespace: string, name: string) =>
  `${apiVersion}|${kind}|${namespace}|${name}`;
