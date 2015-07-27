package sandbox

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/docker/libnetwork/netutils"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

const (
	vethName1     = "wierdlongname1"
	vethName2     = "wierdlongname2"
	vethName3     = "wierdlongname3"
	vethName4     = "wierdlongname4"
	sboxIfaceName = "containername"
)

func newKey(t *testing.T) (string, error) {
	name, err := netutils.GenerateRandomName("netns", 12)
	if err != nil {
		return "", err
	}

	name = filepath.Join("/tmp", name)
	if _, err := os.Create(name); err != nil {
		return "", err
	}

	// Set the rpmCleanupPeriod to be low to make the test run quicker
	gpmLock.Lock()
	gpmCleanupPeriod = 2 * time.Second
	gpmLock.Unlock()

	return name, nil
}

func newInfo(t *testing.T) (*Info, error) {
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: vethName1, TxQLen: 0},
		PeerName:  vethName2}
	if err := netlink.LinkAdd(veth); err != nil {
		return nil, err
	}

	// Store the sandbox side pipe interface
	// This is needed for cleanup on DeleteEndpoint()
	intf1 := &Interface{}
	intf1.SrcName = vethName2
	intf1.DstName = sboxIfaceName

	ip4, addr, err := net.ParseCIDR("192.168.1.100/24")
	if err != nil {
		return nil, err
	}
	intf1.Addresses = addr
	intf1.Addresses.IP = ip4

	// ip6, addrv6, err := net.ParseCIDR("2001:DB8::ABCD/48")
	ip6, addrv6, err := net.ParseCIDR("fe80::2/64")
	if err != nil {
		return nil, err
	}
	intf1.AddressIPv6 = addrv6
	intf1.AddressIPv6.IP = ip6

	veth = &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: vethName3, TxQLen: 0},
		PeerName:  vethName4}

	if err := netlink.LinkAdd(veth); err != nil {
		return nil, err
	}

	intf2 := &Interface{}
	intf2.SrcName = vethName4
	intf2.DstName = sboxIfaceName

	ip4, addr, err = net.ParseCIDR("192.168.2.100/24")
	if err != nil {
		return nil, err
	}
	intf2.Addresses = addr
	intf2.Addresses.IP = ip4

	// ip6, addrv6, err := net.ParseCIDR("2001:DB8::ABCD/48")
	ip6, addrv6, err = net.ParseCIDR("fe80::3/64")
	if err != nil {
		return nil, err
	}
	intf2.AddressIPv6 = addrv6
	intf2.AddressIPv6.IP = ip6

	sinfo := &Info{Interfaces: []*Interface{intf1, intf2}}
	sinfo.Gateway = net.ParseIP("192.168.1.1")
	// sinfo.GatewayIPv6 = net.ParseIP("2001:DB8::1")
	sinfo.GatewayIPv6 = net.ParseIP("fe80::1")

	return sinfo, nil
}

func verifySandbox(t *testing.T, s Sandbox) {
	_, ok := s.(*networkNamespace)
	if !ok {
		t.Fatalf("The sandox interface returned is not of type networkNamespace")
	}

	origns, err := netns.Get()
	if err != nil {
		t.Fatalf("Could not get the current netns: %v", err)
	}
	defer origns.Close()

	f, err := os.OpenFile(s.Key(), os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("Failed top open network namespace path %q: %v", s.Key(), err)
	}
	defer f.Close()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	nsFD := f.Fd()
	if err = netns.Set(netns.NsHandle(nsFD)); err != nil {
		t.Fatalf("Setting to the namespace pointed to by the sandbox %s failed: %v", s.Key(), err)
	}
	defer netns.Set(origns)

	_, err = netlink.LinkByName(sboxIfaceName + "0")
	if err != nil {
		t.Fatalf("Could not find the interface %s inside the sandbox: %v", sboxIfaceName,
			err)
	}

	_, err = netlink.LinkByName(sboxIfaceName + "1")
	if err != nil {
		t.Fatalf("Could not find the interface %s inside the sandbox: %v", sboxIfaceName,
			err)
	}
}

func verifyCleanup(t *testing.T, s Sandbox, wait bool) {
	if wait {
		time.Sleep(time.Duration(gpmCleanupPeriod * 2))
	}

	if _, err := os.Stat(s.Key()); err == nil {
		if wait {
			t.Fatalf("The sandbox path %s is not getting cleaned up even after twice the cleanup period", s.Key())
		} else {
			t.Fatalf("The sandbox path %s is not cleaned up after running gc", s.Key())
		}
	}
}

func TestScanStatistics(t *testing.T) {
	data :=
		"Inter-|   Receive                                                |  Transmit\n" +
			"	face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n" +
			"  eth0:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0\n" +
			" wlan0: 7787685   11141    0    0    0     0          0         0  1681390    7220    0    0    0     0       0          0\n" +
			"    lo:  783782    1853    0    0    0     0          0         0   783782    1853    0    0    0     0       0          0\n" +
			"lxcbr0:       0       0    0    0    0     0          0         0     9006      61    0    0    0     0       0          0\n"

	i := &InterfaceStatistics{}

	if err := scanInterfaceStats(data, "wlan0", i); err != nil {
		t.Fatal(err)
	}
	if i.TxBytes != 1681390 || i.TxPackets != 7220 || i.RxBytes != 7787685 || i.RxPackets != 11141 {
		t.Fatalf("Error scanning the statistics")
	}

	if err := scanInterfaceStats(data, "lxcbr0", i); err != nil {
		t.Fatal(err)
	}
	if i.TxBytes != 9006 || i.TxPackets != 61 || i.RxBytes != 0 || i.RxPackets != 0 {
		t.Fatalf("Error scanning the statistics")
	}
}
