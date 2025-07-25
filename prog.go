package libbpfgo

/*
#cgo LDFLAGS: -lelf -lz
#include "libbpfgo.h"
*/
import "C"

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

//
// BPFProg
//

type BPFProg struct {
	prog       *C.struct_bpf_program
	module     *Module
	pinnedPath string
}

func (p *BPFProg) FileDescriptor() int {
	return int(C.bpf_program__fd(p.prog))
}

// Deprecated: use BPFProg.FileDescriptor() instead.
func (p *BPFProg) GetFd() int {
	return p.FileDescriptor()
}

func (p *BPFProg) Pin(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %s: %v", path, err)
	}

	absPathC := C.CString(absPath)
	defer C.free(unsafe.Pointer(absPathC))

	retC := C.bpf_program__pin(p.prog, absPathC)
	if retC < 0 {
		return fmt.Errorf("failed to pin program %s to %s: %w", p.Name(), path, syscall.Errno(-retC))
	}

	p.pinnedPath = absPath

	return nil
}

func (p *BPFProg) Unpin(path string) error {
	pathC := C.CString(path)
	defer C.free(unsafe.Pointer(pathC))

	retC := C.bpf_program__unpin(p.prog, pathC)
	if retC < 0 {
		return fmt.Errorf("failed to unpin program %s to %s: %w", p.Name(), path, syscall.Errno(-retC))
	}

	p.pinnedPath = ""

	return nil
}

func (p *BPFProg) GetModule() *Module {
	return p.module
}

func (p *BPFProg) Name() string {
	return C.GoString(C.bpf_program__name(p.prog))
}

// Deprecated: use BPFProg.Name() instead.
func (p *BPFProg) GetName() string {
	return p.Name()
}

func (p *BPFProg) SectionName() string {
	return C.GoString(C.bpf_program__section_name(p.prog))
}

// Deprecated: use BPFProg.SectionName() instead.
func (p *BPFProg) GetSectionName() string {
	return p.SectionName()
}

func (p *BPFProg) PinPath() string {
	return p.pinnedPath // There's no LIBBPF_API for bpf program
}

// Deprecated: use BPFProg.PinPath() instead.
func (p *BPFProg) GetPinPath() string {
	return p.PinPath()
}

func (p *BPFProg) GetType() BPFProgType {
	return BPFProgType(C.bpf_program__type(p.prog))
}

func (p *BPFProg) SetAutoload(autoload bool) error {
	retC := C.bpf_program__set_autoload(p.prog, C.bool(autoload))
	if retC < 0 {
		return fmt.Errorf("failed to set bpf program autoload: %w", syscall.Errno(-retC))
	}

	return nil
}

func (p *BPFProg) Autoload() bool {
	return bool(C.bpf_program__autoload(p.prog))
}

func (p *BPFProg) SetAutoattach(autoload bool) {
	C.bpf_program__set_autoattach(p.prog, C.bool(autoload))
}

func (p *BPFProg) Autoattach() bool {
	return bool(C.bpf_program__autoattach(p.prog))
}

// AttachGeneric is used to attach the BPF program using autodetection
// for the attach target. You can specify the destination in BPF code
// via the SEC() such as `SEC("fentry/some_kernel_func")`
func (p *BPFProg) AttachGeneric() (*BPFLink, error) {
	linkC, errno := C.bpf_program__attach(p.prog)
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach program: %w", errno)
	}

	return &BPFLink{
		link:      linkC,
		prog:      p,
		linkType:  Tracing,
		eventName: fmt.Sprintf("tracing-%s", p.Name()),
	}, nil
}

// SetAttachTarget can be used to specify the program and/or function to attach
// the BPF program to. To attach to a kernel function specify attachProgFD as 0
func (p *BPFProg) SetAttachTarget(attachProgFD int, attachFuncName string) error {
	attachFuncNameC := C.CString(attachFuncName)
	defer C.free(unsafe.Pointer(attachFuncNameC))

	retC := C.bpf_program__set_attach_target(p.prog, C.int(attachProgFD), attachFuncNameC)
	if retC < 0 {
		return fmt.Errorf("failed to set attach target for program %s %s %w", p.Name(), attachFuncName, syscall.Errno(-retC))
	}

	return nil
}

// Deprecated: use BPFProg.SetType() instead.
func (p *BPFProg) SetProgramType(progType BPFProgType) {
	_ = p.SetType(progType)
}

func (p *BPFProg) SetType(progType BPFProgType) error {
	retC := C.bpf_program__set_type(p.prog, C.enum_bpf_prog_type(progType))
	if retC < 0 {
		return fmt.Errorf("failed to set prog_type %s for program %s: %w", progType.String(), p.Name(), syscall.Errno(-retC))
	}

	return nil
}

// Deprecated: use BPFProg.SetExpectedAttachType() instead.
func (p *BPFProg) SetAttachType(attachType BPFAttachType) {
	_ = p.SetExpectedAttachType(attachType)
}

func (p *BPFProg) SetExpectedAttachType(attachType BPFAttachType) error {
	retC := C.bpf_program__set_expected_attach_type(p.prog, C.enum_bpf_attach_type(attachType))
	if retC < 0 {
		return fmt.Errorf("failed to set attach_type %s for program %s: %w", attachType.String(), p.Name(), syscall.Errno(-retC))
	}

	return nil
}

// getCgroupDirFD returns a file descriptor for a given cgroup2 directory path
func getCgroupDirFD(cgroupV2DirPath string) (int, error) {
	// revive:disable
	const (
		O_DIRECTORY int = syscall.O_DIRECTORY
		O_RDONLY    int = syscall.O_RDONLY
	)
	// revive:enable

	fd, err := syscall.Open(cgroupV2DirPath, O_DIRECTORY|O_RDONLY, 0)
	if fd < 0 {
		return 0, fmt.Errorf("failed to open cgroupv2 directory path %s: %w", cgroupV2DirPath, err)
	}

	return fd, nil
}

// AttachCgroup attaches the BPFProg to a cgroup described by given fd.
func (p *BPFProg) AttachCgroup(cgroupV2DirPath string) (*BPFLink, error) {
	cgroupDirFD, err := getCgroupDirFD(cgroupV2DirPath)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(cgroupDirFD)

	linkC, errno := C.bpf_program__attach_cgroup(p.prog, C.int(cgroupDirFD))
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach cgroup on cgroupv2 %s to program %s: %w", cgroupV2DirPath, p.Name(), errno)
	}

	// dirName will be used in bpfLink.eventName. eventName follows a format
	// convention and is used to better identify link types and what they are
	// linked with in case of errors or similar needs. Having eventName as:
	// cgroup-progName-/sys/fs/cgroup/unified/ would look weird so replace it
	// to be cgroup-progName-sys-fs-cgroup-unified instead.
	dirName := strings.ReplaceAll(cgroupV2DirPath[1:], "/", "-")

	bpfLink := &BPFLink{
		link:      linkC,
		prog:      p,
		linkType:  Cgroup,
		eventName: fmt.Sprintf("cgroup-%s-%s", p.Name(), dirName),
	}
	p.module.links = append(p.module.links, bpfLink)

	return bpfLink, nil
}

// AttachCgroupLegacy attaches the BPFProg to a cgroup described by the given
// fd. It first tries to use the most recent attachment method and, if that does
// not work, instead of failing, it tries the legacy way: to attach the cgroup
// eBPF program without previously creating a link. This allows attaching cgroup
// eBPF ingress/egress in older kernels. Note: the first attempt error message
// is filtered out inside libbpf_print_fn() as it is actually a feature probe
// attempt as well.
//
// Related kernel commit: https://github.com/torvalds/linux/commit/af6eea57437a
func (p *BPFProg) AttachCgroupLegacy(cgroupV2DirPath string, attachType BPFAttachType) (*BPFLink, error) {
	bpfLink, err := p.AttachCgroup(cgroupV2DirPath)
	if err == nil {
		return bpfLink, nil
	}

	// Try the legacy attachment method before fully failing
	cgroupDirFD, err := getCgroupDirFD(cgroupV2DirPath)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(cgroupDirFD)

	retC, errno := C.cgo_bpf_prog_attach_cgroup_legacy(
		C.int(p.FileDescriptor()),
		C.int(cgroupDirFD),
		C.int(attachType),
	)
	if retC < 0 {
		return nil, fmt.Errorf("failed to attach (legacy) program %s to cgroupv2 %s: %w", p.Name(), cgroupV2DirPath, errno)
	}

	dirName := strings.ReplaceAll(cgroupV2DirPath[1:], "/", "-")

	bpfLinkLegacy := &bpfLinkLegacy{
		attachType: attachType,
		cgroupDir:  cgroupV2DirPath,
	}
	fakeBpfLink := &BPFLink{
		link:      nil, // detach/destroy made with progfd
		prog:      p,
		eventName: fmt.Sprintf("cgroup-%s-%s", p.Name(), dirName),
		// info bellow needed for detach (there isn't a real ebpf link)
		linkType: CgroupLegacy,
		legacy:   bpfLinkLegacy,
	}

	return fakeBpfLink, nil
}

// DetachCgroupLegacy detaches the BPFProg from a cgroup described by the given
// fd. This is needed because in legacy attachment there is no BPFLink, just a
// fake one (kernel did not support it, nor libbpf). This function should be
// called by the (*BPFLink)->Destroy() function, since BPFLink is emulated (so
// users don´t need to distinguish between regular and legacy cgroup
// detachments).
func (p *BPFProg) DetachCgroupLegacy(cgroupV2DirPath string, attachType BPFAttachType) error {
	cgroupDirFD, err := getCgroupDirFD(cgroupV2DirPath)
	if err != nil {
		return err
	}
	defer syscall.Close(cgroupDirFD)

	retC, errno := C.cgo_bpf_prog_detach_cgroup_legacy(
		C.int(p.FileDescriptor()),
		C.int(cgroupDirFD),
		C.int(attachType),
	)
	if retC < 0 {
		return fmt.Errorf("failed to detach (legacy) program %s from cgroupv2 %s: %w", p.Name(), cgroupV2DirPath, errno)
	}

	return nil
}

func (p *BPFProg) AttachXDP(deviceName string) (*BPFLink, error) {
	iface, err := net.InterfaceByName(deviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to find device by name %s: %w", deviceName, err)
	}

	linkC, errno := C.bpf_program__attach_xdp(p.prog, C.int(iface.Index))
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach xdp on device %s to program %s: %w", deviceName, p.Name(), errno)
	}

	bpfLink := &BPFLink{
		link:      linkC,
		prog:      p,
		linkType:  XDP,
		eventName: fmt.Sprintf("xdp-%s-%s", p.Name(), deviceName),
	}
	p.module.links = append(p.module.links, bpfLink)

	return bpfLink, nil
}

func (p *BPFProg) AttachXDPLegacy(deviceName string, flag XDPFlags) error {
	optsC, errno := C.cgo_bpf_xdp_attach_opts_new(C.uint32_t(0))
	if optsC == nil {
		return fmt.Errorf("failed to create xdp attach opts:%w", errno)
	}
	defer C.cgo_bpf_xdp_attach_opts_free(optsC)
	iface, err := net.InterfaceByName(deviceName)
	if err != nil {
		return fmt.Errorf("failed to find device by name %s: %w", deviceName, err)
	}
	var retC C.int
	retC, errno = C.bpf_xdp_attach(C.int(iface.Index), C.int(p.FileDescriptor()), C.uint32_t(flag), optsC)
	if retC < 0 {
		return fmt.Errorf("failed to attach xdp: %w", errno)
	}

	return nil
}

func (p *BPFProg) DetachXDPLegacy(deviceName string, flag XDPFlags) error {
	optsC, errno := C.cgo_bpf_xdp_attach_opts_new(C.uint32_t(p.FileDescriptor()))
	if optsC == nil {
		return fmt.Errorf("failed to create xdp attach opts:%w", errno)
	}
	defer C.cgo_bpf_xdp_attach_opts_free(optsC)
	iface, err := net.InterfaceByName(deviceName)
	if err != nil {
		return fmt.Errorf("failed to find device by name %s: %w", deviceName, err)
	}
	var retC C.int
	retC, errno = C.bpf_xdp_detach(C.int(iface.Index), C.uint32_t(flag), optsC)
	if retC < 0 {
		return fmt.Errorf("failed to detach xdp: %w", errno)
	}

	return nil
}

func (p *BPFProg) AttachTracepoint(category, name string) (*BPFLink, error) {
	tpCategoryC := C.CString(category)
	defer C.free(unsafe.Pointer(tpCategoryC))
	tpNameC := C.CString(name)
	defer C.free(unsafe.Pointer(tpNameC))

	linkC, errno := C.bpf_program__attach_tracepoint(p.prog, tpCategoryC, tpNameC)
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach tracepoint %s to program %s: %w", name, p.Name(), errno)
	}

	bpfLink := &BPFLink{
		link:      linkC,
		prog:      p,
		linkType:  Tracepoint,
		eventName: name,
	}
	p.module.links = append(p.module.links, bpfLink)

	return bpfLink, nil
}

func (p *BPFProg) AttachRawTracepoint(tpEvent string) (*BPFLink, error) {
	tpEventC := C.CString(tpEvent)
	defer C.free(unsafe.Pointer(tpEventC))

	linkC, errno := C.bpf_program__attach_raw_tracepoint(p.prog, tpEventC)
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach raw tracepoint %s to program %s: %w", tpEvent, p.Name(), errno)
	}

	bpfLink := &BPFLink{
		link:      linkC,
		prog:      p,
		linkType:  RawTracepoint,
		eventName: tpEvent,
	}
	p.module.links = append(p.module.links, bpfLink)

	return bpfLink, nil
}

func (p *BPFProg) AttachLSM() (*BPFLink, error) {
	linkC, errno := C.bpf_program__attach_lsm(p.prog)
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach lsm to program %s: %w", p.Name(), errno)
	}

	bpfLink := &BPFLink{
		link:     linkC,
		prog:     p,
		linkType: LSM,
	}
	p.module.links = append(p.module.links, bpfLink)

	return bpfLink, nil
}

func (p *BPFProg) AttachPerfEvent(fd int) (*BPFLink, error) {
	linkC, errno := C.bpf_program__attach_perf_event(p.prog, C.int(fd))
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach perf event to program %s: %w", p.Name(), errno)
	}

	bpfLink := &BPFLink{
		link:     linkC,
		prog:     p,
		linkType: PerfEvent,
	}
	p.module.links = append(p.module.links, bpfLink)

	return bpfLink, nil
}

//
// Kprobe and Kretprobe
//

type attachTo struct {
	symName string
	symAddr uint64
	isRet   bool
}

// attachKprobeCommon is a common function for attaching kprobe and kretprobe.
func (p *BPFProg) attachKprobeCommon(a attachTo) (*BPFLink, error) {
	// Create kprobe_opts.
	optsC, errno := C.cgo_bpf_kprobe_opts_new(
		C.ulonglong(0),      // bpf cookie (not used)
		C.size_t(a.symAddr), // might be 0 if attaching using symbol name
		C.bool(a.isRet),     // is kretprobe or kprobe
		C.int(0),            // attach mode (default)
	)
	if optsC == nil {
		return nil, fmt.Errorf("failed to create kprobe_opts of %v: %v", a, errno)
	}
	defer C.cgo_bpf_kprobe_opts_free(optsC)

	// Create kprobe symbol name.
	symNameC := C.CString(a.symName)
	defer C.free(unsafe.Pointer(symNameC))

	// Create kprobe link.
	var linkC *C.struct_bpf_link
	linkC, errno = C.bpf_program__attach_kprobe_opts(p.prog, symNameC, optsC)
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach to %v: %v", a, errno)
	}

	linkType := Kprobe
	if a.isRet {
		linkType = Kretprobe
	}

	eventName := a.symName
	if eventName == "" {
		eventName = fmt.Sprintf("%d", a.symAddr)
	}

	// Create bpfLink and append it to the module.
	bpfLink := &BPFLink{
		link:      linkC,     // linkC is a pointer to a struct bpf_link
		prog:      p,         // p is a pointer to the related BPFProg
		linkType:  linkType,  // linkType is a BPFLinkType
		eventName: eventName, // eventName is a string
	}
	p.module.links = append(p.module.links, bpfLink)

	return bpfLink, nil
}

// AttachKprobe attaches the BPFProgram to the given symbol name.
func (p *BPFProg) AttachKprobe(symbol string) (*BPFLink, error) {
	a := attachTo{
		symName: symbol,
		isRet:   false,
	}
	return p.attachKprobeCommon(a)
}

// AttachKretprobe attaches the BPFProgram to the given symbol name (for return).
func (p *BPFProg) AttachKretprobe(symbol string) (*BPFLink, error) {
	a := attachTo{
		symName: symbol,
		isRet:   true,
	}
	return p.attachKprobeCommon(a)
}

// AttachKprobeOnOffset attaches the BPFProgram to the given offset.
func (p *BPFProg) AttachKprobeOffset(offset uint64) (*BPFLink, error) {
	a := attachTo{
		symAddr: offset,
		isRet:   false,
	}
	return p.attachKprobeCommon(a)
}

// AttachKretprobeOnOffset attaches the BPFProgram to the given offset (for return).
func (p *BPFProg) AttachKretprobeOnOffset(offset uint64) (*BPFLink, error) {
	a := attachTo{
		symAddr: offset,
		isRet:   true,
	}
	return p.attachKprobeCommon(a)
}

// End of Kprobe and Kretprobe

func (p *BPFProg) AttachNetns(networkNamespacePath string) (*BPFLink, error) {
	fd, err := syscall.Open(networkNamespacePath, syscall.O_RDONLY, 0)
	if fd < 0 {
		return nil, fmt.Errorf("failed to open network namespace path %s: %w", networkNamespacePath, err)
	}

	linkC, errno := C.bpf_program__attach_netns(p.prog, C.int(fd))
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach network namespace on %s to program %s: %w", networkNamespacePath, p.Name(), errno)
	}

	// fileName will be used in bpfLink.eventName. eventName follows a format
	// convention and is used to better identify link types and what they are
	// linked with in case of errors or similar needs. Having eventName as:
	// netns-progName-/proc/self/ns/net would look weird so replace it
	// to be netns-progName-proc-self-ns-net instead.
	fileName := strings.ReplaceAll(networkNamespacePath[1:], "/", "-")

	bpfLink := &BPFLink{
		link:      linkC,
		prog:      p,
		linkType:  Netns,
		eventName: fmt.Sprintf("netns-%s-%s", p.Name(), fileName),
	}
	p.module.links = append(p.module.links, bpfLink)

	return bpfLink, nil
}

type IterOpts struct {
	MapFd           int
	CgroupIterOrder BPFCgroupIterOrder
	CgroupFd        int
	CgroupId        uint64
	Tid             int
	Pid             int
	PidFd           int
}

func (p *BPFProg) AttachIter(opts IterOpts) (*BPFLink, error) {
	optsC, errno := C.cgo_bpf_iter_attach_opts_new(
		C.uint(opts.MapFd),
		uint32(opts.CgroupIterOrder),
		C.uint(opts.CgroupFd),
		C.ulonglong(opts.CgroupId),
		C.uint(opts.Tid),
		C.uint(opts.Pid),
		C.uint(opts.PidFd),
	)
	if optsC == nil {
		return nil, fmt.Errorf("failed to create iter_attach_opts to program %s: %w", p.Name(), errno)
	}
	defer C.cgo_bpf_iter_attach_opts_free(optsC)

	linkC, errno := C.bpf_program__attach_iter(p.prog, optsC)
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach iter to program %s: %w", p.Name(), errno)
	}

	bpfLink := &BPFLink{
		link:      linkC,
		prog:      p,
		linkType:  Iter,
		eventName: fmt.Sprintf("iter-%s-%d", p.Name(), opts.MapFd),
	}
	p.module.links = append(p.module.links, bpfLink)

	return bpfLink, nil
}

// AttachUprobe attaches the BPFProgram to entry of the symbol in the library or binary at 'path'
// which can be relative or absolute. A pid can be provided to attach to, or -1 can be specified
// to attach to all processes
func (p *BPFProg) AttachUprobe(pid int, path string, offset uint64) (*BPFLink, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	return doAttachUprobe(p, false, pid, absPath, offset)
}

// AttachURetprobe attaches the BPFProgram to exit of the symbol in the library or binary at 'path'
// which can be relative or absolute. A pid can be provided to attach to, or -1 can be specified
// to attach to all processes
func (p *BPFProg) AttachURetprobe(pid int, path string, offset uint64) (*BPFLink, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	return doAttachUprobe(p, true, pid, absPath, offset)
}

// AttachUprobeMulti attaches the BPFProgram to entry of the symbol in the library or binary at 'path'
// which can be relative or absolute, using the uprobe_multi link, allowing to specify multiple offsets.
// A pid can be provided to attach to, or -1 can be specified to attach to all processes.
// func_pattern argument to libbpf bpf_program__attach_uprobe_multi() which accepts a regular expression
// to specify functions to attach BPF program to, is currently not supported.
func (p *BPFProg) AttachUprobeMulti(pid int, path string, offsets, cookies []uint64) (*BPFLink, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	return doAttachUprobeMulti(p, false, pid, absPath, offsets, cookies)
}

// AttachURetprobeMulti attaches the BPFProgram to exit of the symbol in the library or binary at 'path'
// which can be relative or absolute, using the uprobe_multi link, allowing to specify multiple offsets.
// A pid can be provided to attach to, or -1 can be specified to attach to all processes.
// func_pattern argument to libbpf bpf_program__attach_uprobe_multi() which accepts a regular expression
// to specify functions to attach BPF program to, is currently not supported.
func (p *BPFProg) AttachURetprobeMulti(pid int, path string, offsets, cookies []uint64) (*BPFLink, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	return doAttachUprobeMulti(p, true, pid, absPath, offsets, cookies)
}

func doAttachUprobe(prog *BPFProg, isUretprobe bool, pid int, path string, offset uint64) (*BPFLink, error) {
	pathC := C.CString(path)
	defer C.free(unsafe.Pointer(pathC))

	linkC, errno := C.bpf_program__attach_uprobe(
		prog.prog,
		C.bool(isUretprobe),
		C.int(pid),
		pathC,
		C.size_t(offset),
	)
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach u(ret)probe to program %s:%d with pid %d: %w ", path, offset, pid, errno)
	}

	upType := Uprobe
	if isUretprobe {
		upType = Uretprobe
	}

	bpfLink := &BPFLink{
		link:      linkC,
		prog:      prog,
		linkType:  upType,
		eventName: fmt.Sprintf("%s:%d:%d", path, pid, offset),
	}

	return bpfLink, nil
}

func doAttachUprobeMulti(prog *BPFProg, isUretprobe bool, pid int, path string, offsets, cookies []uint64) (*BPFLink, error) {
	pathC := C.CString(path)
	defer C.free(unsafe.Pointer(pathC))

	var offsetsC *C.ulong
	if len(offsets) > 0 {
		offsetsC = (*C.ulong)(unsafe.Pointer(&offsets[0]))
	}
	var cookiesC *C.__u64
	if len(cookies) > 0 {
		cookiesC = (*C.__u64)(unsafe.Pointer(&cookies[0]))
	}

	linkC, errno := C.cgo_bpf_program__attach_uprobe_multi(
		prog.prog,
		C.int(pid),
		pathC,
		nil,
		offsetsC,
		cookiesC,
		C.size_t(len(offsets)),
		C.bool(isUretprobe),
	)
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach u(ret)probe multi to program %s:%v with pid %d: %w ", path, offsets, pid, errno)
	}

	upType := Uprobe
	if isUretprobe {
		upType = Uretprobe
	}

	bpfLink := &BPFLink{
		link:      linkC,
		prog:      prog,
		linkType:  upType,
		eventName: fmt.Sprintf("%s:%d:%v", path, pid, offsets),
	}

	return bpfLink, nil
}

// AttachGenericFD attaches the BPFProgram to a targetFd at the specified attachType hook.
func (p *BPFProg) AttachGenericFD(targetFd int, attachType BPFAttachType, flags AttachFlag) error {
	retC := C.bpf_prog_attach(
		C.int(p.FileDescriptor()),
		C.int(targetFd),
		C.enum_bpf_attach_type(int(attachType)),
		C.uint(uint(flags)),
	)
	if retC < 0 {
		return fmt.Errorf("failed to attach: %w", syscall.Errno(-retC))
	}

	return nil
}

// DetachGenericFD detaches the BPFProgram associated with the targetFd at the hook specified by attachType.
func (p *BPFProg) DetachGenericFD(targetFd int, attachType BPFAttachType) error {
	retC := C.bpf_prog_detach2(
		C.int(p.FileDescriptor()),
		C.int(targetFd),
		C.enum_bpf_attach_type(int(attachType)),
	)
	if retC < 0 {
		return fmt.Errorf("failed to detach: %w", syscall.Errno(-retC))
	}

	return nil
}

// AttachUSDT attaches the BPFProgram to the USDT static marker in the library or binary at 'binaryPath'
// which can be relative or absolute. A pid can be provided to attach to, or -1 can be specified
// to attach to all processes.
func (p *BPFProg) AttachUSDT(pid int, binaryPath string, provider string, name string) (*BPFLink, error) {
	binaryPathC := C.CString(binaryPath)
	defer C.free(unsafe.Pointer(binaryPathC))
	providerC := C.CString(provider)
	defer C.free(unsafe.Pointer(providerC))
	nameC := C.CString(name)
	defer C.free(unsafe.Pointer(nameC))

	linkC, errno := C.bpf_program__attach_usdt(
		p.prog,
		C.int(pid),
		binaryPathC,
		providerC,
		nameC,
		nil,
	)
	if linkC == nil {
		return nil, fmt.Errorf("failed to attach USDT probe to marker %s:%s in program %s with pid %d: %w", provider, name, binaryPath, pid, errno)
	}

	bpfLink := &BPFLink{
		link:      linkC,
		prog:      p,
		linkType:  USDT,
		eventName: fmt.Sprintf("%s:%d:%s:%s", binaryPath, pid, provider, name),
	}

	return bpfLink, nil
}

//
// BPF_PROG_TEST_RUN
//

type RunFlag uint32

const (
	RunFlagRunOnCPU      RunFlag = C.BPF_F_TEST_RUN_ON_CPU
	RunFlagXDPLiveFrames RunFlag = C.BPF_F_TEST_XDP_LIVE_FRAMES
)

// RunOpts mirrors the C structure bpf_test_run_opts.
type RunOpts struct {
	DataIn      []byte
	DataOut     []byte
	DataSizeIn  uint32
	DataSizeOut uint32
	CtxIn       []byte
	CtxOut      []byte
	CtxSizeIn   uint32
	CtxSizeOut  uint32
	RetVal      uint32
	Repeat      int
	Duration    time.Duration
	Flags       RunFlag
	CPU         uint32
	BatchSize   uint32
}

func runOptsToC(runOpts *RunOpts) (*C.struct_bpf_test_run_opts, error) {
	if runOpts == nil {
		return nil, nil
	}

	var (
		dataIn      unsafe.Pointer
		dataSizeIn  C.uint
		dataOut     unsafe.Pointer
		dataSizeOut C.uint
		ctxIn       unsafe.Pointer
		ctxSizeIn   C.uint
		ctxOut      unsafe.Pointer
		ctxSizeOut  C.uint
	)

	if runOpts.DataIn != nil {
		dataIn = unsafe.Pointer(&runOpts.DataIn[0])
		dataSizeIn = C.uint(runOpts.DataSizeIn)
	}
	if runOpts.DataOut != nil {
		dataOut = unsafe.Pointer(&runOpts.DataOut[0])
		dataSizeOut = C.uint(runOpts.DataSizeOut)
	}
	if runOpts.CtxIn != nil {
		ctxIn = unsafe.Pointer(&runOpts.CtxIn[0])
		ctxSizeIn = C.uint(runOpts.CtxSizeIn)
	}
	if runOpts.CtxOut != nil {
		ctxOut = unsafe.Pointer(&runOpts.CtxOut[0])
		ctxSizeOut = C.uint(runOpts.CtxSizeOut)
	}
	optsC, errno := C.cgo_bpf_test_run_opts_new(
		dataIn,
		dataOut,
		dataSizeIn,
		dataSizeOut,
		ctxIn,
		ctxOut,
		ctxSizeIn,
		ctxSizeOut,
		C.int(runOpts.Repeat),
		C.uint(runOpts.Flags),
		C.uint(runOpts.CPU),
		C.uint(runOpts.BatchSize),
	)
	if optsC == nil {
		return nil, fmt.Errorf("failed to create bpf_test_run_opts: %w", errno)
	}

	return optsC, nil
}

func runOptsFromC(runOpts *RunOpts, optsC *C.struct_bpf_test_run_opts) {
	if optsC == nil {
		return
	}

	if optsC.data_in != nil {
		runOpts.DataIn = C.GoBytes(optsC.data_in, C.int(optsC.data_size_in))
	}
	if optsC.data_out != nil {
		runOpts.DataOut = C.GoBytes(optsC.data_out, C.int(optsC.data_size_out))
	}
	if optsC.ctx_in != nil {
		runOpts.CtxIn = C.GoBytes(optsC.ctx_in, C.int(optsC.ctx_size_in))
	}
	if optsC.ctx_out != nil {
		runOpts.CtxOut = C.GoBytes(optsC.ctx_out, C.int(optsC.ctx_size_out))
	}

	runOpts.RetVal = uint32(optsC.retval)
	runOpts.Repeat = int(optsC.repeat)
	runOpts.Duration = time.Duration(optsC.duration) * time.Nanosecond
	runOpts.Flags = RunFlag(optsC.flags)
	runOpts.CPU = uint32(optsC.cpu)
	runOpts.BatchSize = uint32(optsC.batch_size)
}

// Run executes the eBPF program without attaching it to actual hooks, filling
// the results in the provided RunOpts.
// Reference:
//   - https://docs.kernel.org/bpf/bpf_prog_run.html
//   - https://docs.kernel.org/userspace-api/ebpf/syscall.html
//
// Example Usage:
//
//	/*
//	SEC("tc")
//	int test(struct __sk_buff *skb)
//	{
//	    return foo() ? 1 : 0;
//	}
//	*/
//
//	func TestFunc(t *testing.T) {
//	    ...
//	    prog, _ := module.GetProgram("test")
//	    opts := RunOpts{
//	        DataIn: make([]byte, 0, 14),
//	        DataSizeIn: 14,
//	        DataOut: make([]byte, 0, 14),
//	        DataSizeOut: 14,
//	        Repeat: 1,
//	    }
//	    prog.Run(&opts)
//	    if opts.RetVal != 1 {
//	        t.Errorf("result = %d; want 1", opts.RetVal)
//	    }
//	}
func (p *BPFProg) Run(opts *RunOpts) error {
	optsC, err := runOptsToC(opts)
	if err != nil {
		return err
	}
	defer C.cgo_bpf_test_run_opts_free(optsC)

	retC := C.bpf_prog_test_run_opts(C.int(p.FileDescriptor()), optsC)
	if retC < 0 {
		return fmt.Errorf("failed to run program: %w", syscall.Errno(-retC))
	}

	// update runOpts with the values from the kernel and libbpf
	runOptsFromC(opts, optsC)

	return nil
}
