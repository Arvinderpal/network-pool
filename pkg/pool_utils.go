package pool

import "net"

// generates a subnet from the specified prefix and subnet bits
func computeSubnetFromOffset(prefix net.IP, prefixLength, subnetLength int, subnetBits uint32) net.IP {

	ipUInt32 := packIPv4IntoUint32(prefix)
	ipUInt32 = packBits(ipUInt32, prefixLength, subnetBits, subnetLength)
	return unpackUint32ToIPv4(ipUInt32)
}

// generates a mask of the specified length
func computeSubnetMask(length int) net.IPMask {
	return unpackUint32ToIPv4Mask(uint32(0xFFFFFFFF) << uint(length))
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
