// Package credentialprovider contains a copy of some of the files in
// https://github.com/kubernetes/kubernetes/tree/97332c1edca5be0082414d8a030a408f91bed003/pkg/credentialprovider
//
// The above library is supported nor recommended for use as a module/dependency, see
// https://github.com/kubernetes/kubernetes/issues/79384#issuecomment-505627280 and
// https://github.com/kubernetes/kubernetes/#to-start-using-k8s
//
// Therefore we have copy of the functionality necessary to use pull secrets the same way as kubernetes does.
// The files we copied do not change often upstream, but ideally we should check for changes every kubernetes release
// and update the permalink above to reflect the latest sync point.
package credentialprovider
