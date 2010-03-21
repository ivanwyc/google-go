// mksysnum_nacl.sh /home/rsc/pub/nacl/native_client/src/trusted/service_runtime/include/bits/nacl_syscalls.h
// MACHINE GENERATED BY THE ABOVE COMMAND; DO NOT EDIT

package syscall

// TODO(brainman): autogenerate winapi proc pointers in zsysnum_mingw.go

var (
	SYS_KERNEL32         = loadDll("kernel32.dll")
	SYS_GET_LAST_ERROR   = getSysProcAddr(SYS_KERNEL32, "GetLastError")
	SYS_LOAD_LIBRARY_A   = getSysProcAddr(SYS_KERNEL32, "LoadLibraryA")
	SYS_FREE_LIBRARY     = getSysProcAddr(SYS_KERNEL32, "FreeLibrary")
	SYS_GET_PROC_ADDRESS = getSysProcAddr(SYS_KERNEL32, "GetProcAddress")
	SYS_GET_VERSION      = getSysProcAddr(SYS_KERNEL32, "GetVersion")
)