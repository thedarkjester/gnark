pragma solidity ^0.8.0;
pragma experimental ABIEncoderV2;

/**
 * @title UtilsFr
 * @dev Hash msg following https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-06#section-5.2v
 *      matches github.com/consensys/gnark-crypto/ecc/bn254/fr's hash function, with count=1
 */
library UtilsFr {

  /**
  * @dev Returns the length of a given string
  *
  * @param s The string to measure the length of
  * @return The length of the input string
  */
  function strlen(string memory s) internal pure returns (uint256) {
    uint256 len;
    uint256 i = 0;
    uint256 bytelength = bytes(s).length;
    for (len = 0; i < bytelength; len++) {
      bytes1 b = bytes(s)[i];
      if (b < 0x80) {
          i += 1;
      } else if (b < 0xE0) {
          i += 2;
      } else if (b < 0xF0) {
          i += 3;
      } else if (b < 0xF8) {
          i += 4;
      } else if (b < 0xFC) {
          i += 5;
      } else {
          i += 6;
      }
    }
    return len;
  }
  
  /**
   * @dev ExpandMsgXmd expands msg to a slice of lenInBytes bytes.
   *      https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-06#section-5
   *      https://tools.ietf.org/html/rfc8017#section-4.1 (I2OSP/O2ISP)
   */
  function expand_msg(uint256 x, uint256 y, string memory dst) public pure returns(uint8[48] memory res){
    
    //uint8[64] memory pad; // 64 is sha256 block size.
    // sha256(pad || msg || (0 || 48 || 0) || dst || 11)
    bytes memory tmp;
    uint8 zero = 0;
    uint8 lenInBytes = 48;
    uint8 sizeDomain = uint8(strlen(dst));
    for (uint i=0; i<64; i++){
      tmp = abi.encodePacked(tmp, zero);
    }
    tmp = abi.encodePacked(tmp, x, y, zero, lenInBytes, zero, dst, sizeDomain);
    bytes32 b0 = sha256(tmp);
    
    tmp = abi.encodePacked(b0, uint8(1), dst, sizeDomain);
    bytes32 b1 = sha256(tmp);
    for (uint i=0; i<32; i++){
      res[i] = uint8(b1[i]);
    }

    tmp = abi.encodePacked(uint8(b0[0]) ^ uint8(b1[0]));
    for (uint i=1; i<32; i++){
      tmp = abi.encodePacked(tmp, uint8(b0[i]) ^ uint8(b1[i]));
    }

    tmp = abi.encodePacked(tmp, uint8(2), dst, sizeDomain);
    b1 = sha256(tmp);

    // TODO handle the size of the dst (check gnark-crypto)
    for (uint i=0; i<16; i++){
      res[i+32] = uint8(b1[i]);
    }

    return res;
  }

  /**
   * @dev cf https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-06#section-5.2
   * corresponds to https://github.com/ConsenSys/gnark-crypto/blob/develop/ecc/bn254/fr/element.go
   */
  function hash_fr(uint256 x, uint256 y, string memory dst) internal pure returns(uint256 res) {

    uint256 r_mod = 21888242871839275222246405745257275088548364400416034343698204186575808495617;

    // interpret a as a bigEndian integer and reduce it mod r
    uint8[48] memory xmsg = expand_msg(x, y, dst);
    // uint8[48] memory xmsg = [0x44, 0x74, 0xb5, 0x29, 0xd7, 0xfb, 0x29, 0x88, 0x3a, 0x7a, 0xc1, 0x65, 0xfd, 0x72, 0xce, 0xd0, 0xd4, 0xd1, 0x3f, 0x9e, 0x85, 0x8a, 0x3, 0x86, 0x1c, 0x90, 0x83, 0x1e, 0x94, 0xdc, 0xfc, 0x1d, 0x70, 0x82, 0xf5, 0xbf, 0x30, 0x3, 0x39, 0x87, 0x21, 0x38, 0x15, 0xed, 0x12, 0x75, 0x44, 0x6a];
   
   // reduce xmsg mod r, where xmsg is intrepreted in big endian 
   // (as SetBytes does for golang's Big.Int library).
    for (uint i=0; i<32; i++){
      res += uint256(xmsg[47-i])<<(8*i);
    }
    res = res % r_mod;
    uint256 tmp;
    for (uint i=0; i<16; i++){
      tmp += uint256(xmsg[15-i])<<(8*i);
    }

    // 2**256%r
    uint256 b = 6350874878119819312338956282401532410528162663560392320966563075034087161851; 

    tmp = mulmod(tmp, b, r_mod);
    res = addmod(res, tmp, r_mod);

    return res;
  }

}
