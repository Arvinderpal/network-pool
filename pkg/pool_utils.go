package pool

import (
	"net"

	"github.com/apcera/continuum/util/netutil"
)

// generates a subnet from the specified prefix and subnet bits
func computeSubnetFromOffset(prefix net.IP, prefixLength, subnetLength int, subnetBits uint32) net.IP {
	ipUInt32 := packIPv4IntoUint32(prefix)
	ipUInt32 = packBits(ipUInt32, prefixLength, subnetBits, subnetLength)
	return unpackUint32ToIPv4(ipUInt32)
}

// generates a mask of the specified length
func computeSubnetMask(length int) net.IPMask {
	return unpackUint32ToIPv4Mask(uint32(0xFFFFFFFF) << uint(32-length))
}

func computeOffsetFromSubnet(ip net.IP, prefixLength, subnetLength int) uint {
	tmpOffsetU32 := packIPv4IntoUint32(ip)
	var subnetMask uint32
	subnetMask = (0xFFFFFFFF << uint(prefixLength)) >> uint(prefixLength)
	return uint((tmpOffsetU32 & subnetMask) >> uint(32-(prefixLength+subnetLength)))
}

func packIPv4IntoUint32(ip net.IP) uint32 {
	var tmp uint32
	tmp = uint32(ip[15])
	tmp = tmp | uint32(ip[14])<<8
	tmp = tmp | uint32(ip[13])<<16
	tmp = tmp | uint32(ip[12])<<24
	return tmp
}

func unpackUint32ToIPv4(in uint32) net.IP {
	return net.IPv4(byte(in>>24), byte(in>>16), byte(in>>8), byte(in))
}

func unpackUint32ToIPv4Mask(in uint32) net.IPMask {
	return net.IPv4Mask(byte(in>>24), byte(in>>16), byte(in>>8), byte(in))
}

func packBits(org uint32, org_size int, in uint32, in_size int) uint32 {
	mask := uint32(0xFFFFFFFF)
	in = in << uint(32-(org_size+in_size))
	mask = mask << uint(32-(org_size+in_size))
	return (org | in) & mask
}

func allocateBitVector(maxNetworks int) *netutil.BitVector {
	var bytesNeeded int
	if maxNetworks%8 == 0 {
		bytesNeeded = maxNetworks / 8
	} else {
		bytesNeeded = (maxNetworks / 8) + 1
	}

	bv := &netutil.BitVector{}
	bv.Data = make([]byte, bytesNeeded)
	bv.Length = maxNetworks
	return bv
}

// subnetBitsNeeded is a util method to find the number of bits need for the
// subnet portion of the ipv4 address. for example, if the user desires, 5
// networks, we will need 3 bits; even with 8 networks, we will need 3 bits;
// however, if 9 networks are specified, we need 4 bits.
func subnetBitsNeeded(maxNetworks int) int {
	bitsNeeded := 0
	tmp := maxNetworks
	for {
		if tmp <= 1 {
			break
		}
		tmp = tmp >> 1
		bitsNeeded++
	}
	tmpMaxNetworks := 1 << uint(bitsNeeded)
	if maxNetworks <= tmpMaxNetworks {
		// maxnetworks is an exact power of 2
		// e.g. maxNetworks=4; we need only need 2 bits to handle this
		return bitsNeeded
	} else {
		// e.g. maxNetworks=5, we need 3 bits to handle this
		bitsNeeded++
		return bitsNeeded
	}
}
