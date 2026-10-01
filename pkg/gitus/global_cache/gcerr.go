package global_cache

import (
	"errors"
)

var ErrGlobalCacheTypeNotSupported = errors.New("GLOBAL_CACHE_TYPE_NOT_SUPPORTED: The specified type of global cache is currently not supported")
var ErrKeyNotExist = errors.New("KEY_NOT_EXIST: The requested key is not in the global cache.")

