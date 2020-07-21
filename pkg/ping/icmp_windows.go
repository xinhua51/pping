// +build windows

package ping

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"
)

type ip_option_information struct {
	ttl        uint8
	tos        uint8
	flags      uint8
	optionsize uint8
	optiondata uintptr
}

type icmpv6_echo_reply struct {
	address       ipv6_address_ex
	status        uint32
	roundtriptime uint32
}

type icmp_echo_reply struct {
	address       [4]byte
	status        uint32
	roundtriptime uint32
	datasize      uint16
	reserved      uint16
	data          uintptr
	option        ip_option_information
}

type ipv6_address_ex struct {
	sin6_port     uint16
	sin6_flowinfo uint32
	sin6_addr     [16]byte
	sin6_scope_id uint32
}

var (
	iphlpapi = syscall.MustLoadDLL("iphlpapi.dll")

	icmp6CreateFile = iphlpapi.MustFindProc("Icmp6CreateFile")
	icmp6SendEcho2  = iphlpapi.MustFindProc("Icmp6SendEcho2")

	icmpCreateFile = iphlpapi.MustFindProc("IcmpCreateFile")
	icmpSendEcho2  = iphlpapi.MustFindProc("IcmpSendEcho2")

	icmpCloseHandle = iphlpapi.MustFindProc("IcmpCloseHandle")
)

func (this *IcmpPing) ping_rootless(ctx context.Context) IPingResult {
	ip, isipv6, err := this.parseip()
	if err != nil {
		return this.errorResult(err)
	}
	var handle uintptr = 0xffffffff
	defer func() {
		if handle != 0xffffffff {
			IcmpCloseHandle(handle)
		}
	}()
	if isipv6 {
		handle = Icmp6CreateFile()
		if handle == 0xffffffff {
			return this.errorResult(errors.New("IcmpCreateFile failed"))
		}
		data := make([]byte, 32)
		recv := Icmp6SendEcho(handle, ip, data, this.Timeout)
		if recv == nil {
			return this.errorResult(errors.New("IcmpSendEcho failed"))
		}
		recvmsg := (*icmpv6_echo_reply)(unsafe.Pointer(&recv[0]))
		if recvmsg.status != 0 {
			return this.errorResult(errors.New(fmt.Sprintf("failed with error %d", recvmsg.status)))
		}
		return &IcmpPingResult{
			Time: int(recvmsg.roundtriptime),
			TTL:  -1,
			IP:   recvmsg.address.sin6_addr[:],
		}
	} else {
		handle = IcmpCreateFile()
		if handle == 0xffffffff {
			return this.errorResult(errors.New("IcmpCreateFile failed"))
		}
		data := make([]byte, 32)
		recv := IcmpSendEcho(handle, ip, data, this.Timeout)
		if recv == nil {
			return this.errorResult(errors.New("IcmpSendEcho failed"))
		}
		recvmsg := (*icmp_echo_reply)(unsafe.Pointer(&recv[0]))
		if recvmsg.status != 0 {
			return this.errorResult(errors.New(fmt.Sprintf("failed with error %d", recvmsg.status)))
		}
		return &IcmpPingResult{
			Time: int(recvmsg.roundtriptime),
			TTL:  int(recvmsg.option.ttl),
			IP:   recvmsg.address[:],
		}
	}
}

func ipv4ToInt(ip net.IP) uint32 {
	return binary.LittleEndian.Uint32(ip.To4())
}

func IcmpCreateFile() uintptr {
	h, _, _ := icmpCreateFile.Call()
	return h
}

func IcmpCloseHandle(h uintptr) uintptr {
	ret, _, _ := icmpCloseHandle.Call(h)
	return ret
}

func IcmpSendEcho(handle uintptr, ip net.IP, data []byte, timeout time.Duration) []byte {
	buf := make([]byte, 1500)
	n, _, _ := icmpSendEcho2.Call(
		handle,                            // icmphandle
		0,                                 // event
		0,                                 // apcroutine
		0,                                 // apccontext
		uintptr(ipv4ToInt(ip)),            // destinationaddress
		uintptr(unsafe.Pointer(&data[0])), // requestdata
		uintptr(len(data)),                // requestsize
		0,                                 // requestoptions
		uintptr(unsafe.Pointer(&buf[0])),  // replaybuffer
		uintptr(len(buf)),                 // replysize
		uintptr(timeout.Milliseconds()),   // timeout
	)
	if n == 0 {
		return nil
	}
	return buf[:n]
}

func Icmp6CreateFile() uintptr {
	h, _, _ := icmp6CreateFile.Call()
	return h
}

func Icmp6SendEcho(handle uintptr, ip net.IP, data []byte, timeout time.Duration) []byte {
	ip6source := syscall.RawSockaddrInet6{
		Family: syscall.AF_INET6,
	}
	ip6dest := syscall.RawSockaddrInet6{
		Family: syscall.AF_INET6,
	}
	copy(ip6dest.Addr[:], ip)
	buf := make([]byte, 1500)
	n, _, _ := icmp6SendEcho2.Call(
		handle,                              // icmphandle
		0,                                   // event
		0,                                   // apcroutine
		0,                                   // apccontext
		uintptr(unsafe.Pointer(&ip6source)), // sourceaddress
		uintptr(unsafe.Pointer(&ip6dest)),   // destinationaddress
		uintptr(unsafe.Pointer(&data[0])),   // requestdata
		uintptr(len(data)),                  // requestsize
		0,                                   // requestoptions
		uintptr(unsafe.Pointer(&buf[0])),    // replaybuffer
		uintptr(len(buf)),                   // replysize
		uintptr(timeout.Milliseconds()),     // timeout
	)
	if n == 0 {
		return nil
	}
	return buf[:n]
}
