pragma solidity ^0.8.0;
pragma experimental ABIEncoderV2;

import {Polynomials} from '../crypto/Polynomials.sol';
import {PlonkVerifier} from '../Verifier.sol';
import {Types} from '../crypto/Types.sol';
import {TranscriptLibrary} from '../crypto/Transcript.sol';
import {Fr} from '../crypto/Fr.sol';
import {UtilsFr} from '../crypto/HashFr.sol';
import {Bn254} from '../crypto/Bn254.sol';
import {Kzg} from '../crypto/Kzg.sol';

contract TestContract {

  using Polynomials for *;
  using PlonkVerifier for *;
  using Types for *;
  using TranscriptLibrary for *;
  using Fr for *;
  using Bn254 for *;
  using Kzg for *;

  // function test_hash(uint256 x, uint256 y, string memory dst) public returns(uint256 res){

  //   res = UtilsFr.hash_fr(x, y, dst);

  //   emit PrintUint256(res);

  //   return res;

  // }

  // function test_eval_ith_lagrange(uint256 i, uint256 z, uint256 w, uint256 n) public returns (uint256 res){

  //   res = Polynomials.compute_ith_lagrange_at_z(i, z, w, n);

  //   emit PrintUint256(res);

  // }

  // function test_compute_sum_li_zi(uint256[] memory inputs, uint256 z, uint256 w, uint256 n) public returns (uint256 res){

  //   res = Polynomials.compute_sum_li_zi(inputs, z, w, n);

  //   emit PrintUint256(res);

  // }

  event PrintBool(bool a);
  event PrintUint256(uint256 a);

  function test_assembly() public {
  
    Kzg.OpeningProof[] memory proof = new Kzg.OpeningProof[](2);
    proof[0].h_x = 10;
    proof[0].h_y = 20;
    proof[0].claimed_value = 30;
    proof[1].h_x = 40;
    proof[1].h_y = 50;
    proof[1].claimed_value = 60;
    
    uint256[] memory s = new uint256[](10);
    assembly {
      for {let i:=0} lt(i,10) {i:=add(i,1)}
      {
        mstore(add(s,add(0x20,mul(0x20,i))), mload(add(proof, mul(0x20,i))))
        // ss := mload(add(proof, 0xa0))
      }
    }
    // emit PrintUint256(ss);
    for (uint i=0; i<10; i++){
      emit PrintUint256(s[i]);
    }
  }

  function test_batch_invert(uint256[] memory inputs) public {
    emit PrintUint256(12321);
    uint256[] memory res = Fr.batch_inverse(inputs);
    for (uint i = 0; i < res.length; i++) {
      res[i] = Fr.mul(inputs[i], res[i]);
      emit PrintUint256(res[i]);
    }
  }

  function test_batch_compute_lagrange(uint256 k, uint256 z, uint256 w, uint256 n) public {
    emit PrintUint256(1001001);

    uint256[] memory got = Polynomials.batch_compute_lagranges_at_z(k, z, w, n);
    emit PrintUint256(13579);
    /*for (uint i = 0; i < k; i++) {
      emit PrintUint256(got[i]);
    }*/
    for (uint i = 0; i < k; i++) {
      uint256 want = Polynomials.compute_ith_lagrange_at_z(i, z, w, n);
      emit PrintUint256(Fr.sub(got[i], want));
    }
  }
  function test_plonk_vanilla() public returns(bool) {

    Types.Proof memory proof;
    Types.VerificationKey memory vk;
    
    // uint256[] memory public_inputs = new uint256[](1);

    // public_inputs[0] = 35;
    
    // proof.wire_commitments[0].X = 7402952568124454645845736777641866552675031262050682573556534534858027996784;
    // proof.wire_commitments[0].Y = 7721623742789196166730813243887735477743597239585390242724094247354637565544;
    // proof.wire_commitments[1].X = 11271319237270981124663589355586188807156928810499812473685494720888794872581;
    // proof.wire_commitments[1].Y = 20488811762796854925372757945147232320672748486976170402778457422570786638669;
    // proof.wire_commitments[2].X = 3669172208412190819843581801519336747022181412463804565137782762858611893985;
    // proof.wire_commitments[2].Y = 12448601402265162446888472249857519465227846767809916563399273420395263941915;
    // proof.wire_commitments[3].X = 0;
    // proof.wire_commitments[3].Y = 0;
    // proof.grand_product_commitment.X = 10092250402245204252603902206337034381601405644605884905846783842996521520338;
    // proof.grand_product_commitment.Y = 16731131268265064815566220696608448043796483271291871887273464663755856610927;
    // proof.quotient_poly_commitments[0].X = 21624049071107194757409003813993956585155916097105560254252927000685722430025;
    // proof.quotient_poly_commitments[0].Y = 21322056886658342106307841170159195796496336229423950378264534552603828690248;
    // proof.quotient_poly_commitments[1].X = 1208531179388220327920535913035865384608283306009540378841158842283433451078;
    // proof.quotient_poly_commitments[1].Y = 8337872203508192174552956761531420012950393008003249907492668844815888262281;
    // proof.quotient_poly_commitments[2].X = 21314605163049482422917438086914657743577333949189455562012535945980342078642;
    // proof.quotient_poly_commitments[2].Y = 9753312925837168187675089732387929414977192564019269170890472844836508904954;
    // proof.wire_values_at_zeta[0] = 12420035679103689381146924546067053421694751509495013905243274009204064636882;
    // proof.wire_values_at_zeta[1] = 14433353910868022028400533619512372250663285451644383160429974253654837987957;
    // proof.wire_values_at_zeta[2] = 7045091301142048913570423041906665771589372546780363419502056230232909157408;
    // proof.grand_product_at_zeta_omega = 3541181423555613302668737085131204462813646128736337739223439001793094463637;
    // proof.quotient_polynomial_at_zeta = 4335188781339503573682158301704411587457304474953632704907173941714674708508;
    // proof.linearization_polynomial_at_zeta = 12873239272909347293147728869734547946130640225154416133598374928927260907146;
    // proof.qcprime_at_zeta = 0;
    // proof.permutation_polynomials_at_zeta[0] = 2768527762105052023875300543219346509474203912803109071052099382848818148643;
    // proof.permutation_polynomials_at_zeta[1] = 11999849468532410321943668282153704776854567977933773371563737676235303570120;
    // proof.opening_at_zeta_proof.X = 13546448907362513257769833130391030885597927638789662456112236347751864392466;
    // proof.opening_at_zeta_proof.Y = 17324005865144217255228312393378444114800639813215319488231606200402652504685;
    // proof.opening_at_zeta_omega_proof.X = 7311127190197599671097904572559409683938391656917282863665049102691445214771;
    // proof.opening_at_zeta_omega_proof.Y = 3775341651408032132890886088980972458765236819963905710844910644472534329197;

    // vk.domain_size = 8;
    // vk.omega = 19540430494807482326159819597004422086093766032135589407132600596362845576832;
    // vk.selector_commitments[0].X = 8189666426043331144155669872300600774171192646548141390348956113753612826659;
    // vk.selector_commitments[0].Y = 10313117252544255973189350009555804999945079563194643552452902111721448442971;
    // vk.selector_commitments[1].X = 2884297688615897926157558081052017092039007805580174275059512429510521353270;
    // vk.selector_commitments[1].Y = 18811799544315619829036734559482969749919233915425590430946441872725361598151;
    // vk.selector_commitments[2].X = 11626619589014720822672641922098051869770015499639519338181546071029924308662;
    // vk.selector_commitments[2].Y = 6083678428235555399679843987710050532979061766999254968451629002987536413913;
    // vk.selector_commitments[3].X = 10686584549975741631212192436688876235893754089269055574885255861471560945658;
    // vk.selector_commitments[3].Y = 2633808041567920234861550499108507191305965241418972615251743324312142488233;
    // vk.selector_commitments[4].X = 7758877141323307678959418497588606535400022377604587810029991983861809232258;
    // vk.selector_commitments[4].Y = 14909344509206972948079751360668078630652329581726778118559443254943032200179;
    // vk.selector_commitments[5].X = 0;
    // vk.selector_commitments[5].Y = 0;
    // vk.permutation_commitments[0].X = 11259971107737398289358431927631024480842492775743490333163789640913767061403;
    // vk.permutation_commitments[0].Y = 2311338094084049130958226103528626747959868092981732991251380716666844066408;
    // vk.permutation_commitments[1].X = 10192295337750345373342738789931108868102263759411795958006016848582599001259;
    // vk.permutation_commitments[1].Y = 5862535421258031008351007678455106726734513536108098615905368693160490744826;
    // vk.permutation_commitments[2].X = 18629873756620873235671635932713462746784965955369570126916283160403064656283;
    // vk.permutation_commitments[2].Y = 11408802041040746443363674818850983028977499452386819044888810122061678223118;
    // vk.coset_shift = 5;
    // vk.permutation_non_residues[0] = 5;
    // vk.permutation_non_residues[1] = 25;
    // vk.g2_x.X[0] = 10502847900728352820104995430384591572235862434148733107155956109347693984589;
    // vk.g2_x.X[1] = 14227438095234809947593477115205615798437098135983661833593245518598873470133;
    // vk.g2_x.Y[0] = 21715068306295773599916956786074008492685752252069347482027975832766446299128;
    // vk.g2_x.Y[1] = 7327864992410983220565967131396496522982024563883331581506589780450237498081;
    // vk.commitmentIndex = 0;

    uint256[] memory public_inputs = new uint256[](3);
    public_inputs[0] = 6;
    public_inputs[1] = 7;
    public_inputs[2] = 8;

    proof.wire_commitments[0].X = 15136993133195984427146198656723976006016155651448836712029011094324350716138;
    proof.wire_commitments[0].Y = 14459340593279620571863508149384177746249983937035550477949458936596373965107;
    proof.wire_commitments[1].X = 21003232367709372501451837430898797278780185517655218895087485127513957219222;
    proof.wire_commitments[1].Y = 3824868253843571833375362829100465281829818047450051728199066835761275859289;
    proof.wire_commitments[2].X = 20052680778575398443396546624549987202887743634883688392825045060439187808295;
    proof.wire_commitments[2].Y = 14015482909472906087860468458774003114410351817454316564117646652364197944144;
    proof.wire_committed_commitments = new Bn254.G1Point[](1);
    proof.wire_committed_commitments[0].X =  15552480929910802332205434009033658529300932452690724290794850059288694568607;
    proof.wire_committed_commitments[0].Y =  4106131824108708883155969901844611304524714469488967324682353598451772881497;
    proof.grand_product_commitment.X = 8129382300249911509490994003768166931158456454519181570771543561021078604275;
    proof.grand_product_commitment.Y = 16152052816489514765957359820447985214657491258890001545796749692715439650299;
    proof.quotient_poly_commitments[0].X = 20322267595253153758698719677231418675569873881549989484761886683610366417437;
    proof.quotient_poly_commitments[0].Y = 9878571745392382940810963608086360874234150563732267813042586933182767510138;
    proof.quotient_poly_commitments[1].X = 10187780944047721763399836028129649392015814094297062016647984900675121198477;
    proof.quotient_poly_commitments[1].Y = 17932641774430901128841566993464166907161121085974443041096056905093366102177;
    proof.quotient_poly_commitments[2].X = 18457590569346495976812666189406939977730202462176974574513696424592726780082;
    proof.quotient_poly_commitments[2].Y = 6482702839552877652030428600624199861900963835448886114848087524836661646598;
    proof.wire_values_at_zeta[0] = 5147914084083247636555741113909432788023238731935224321705970839013049335098;
    proof.wire_values_at_zeta[1] = 11285072319599834721395808518851876001078585459194050444352925712060413368187;
    proof.wire_values_at_zeta[2] = 9743976338769318804724056599463012978117527881438551826928791374927337492860;
    proof.grand_product_at_zeta_omega = 4593806665053773110798740279726341994234187426202123910303846961813084696792;
    proof.quotient_polynomial_at_zeta = 316735932536137588220581511099302037891028579128547889996388785546506882698;
    proof.linearization_polynomial_at_zeta = 19350585134276416869177653874145854863819129994210395496841863513331019644994;
    proof.selector_commit_api_at_zeta = new uint256[](1);
    proof.selector_commit_api_at_zeta[0] = 3037506189426785371747045033080583929261182816576630524423545100817866974469;
    proof.permutation_polynomials_at_zeta[0] = 6190498314569077887779183249877684324945647094815852142606141315731906685034;
    proof.permutation_polynomials_at_zeta[1] = 5831053470298471998223188926028283472689709068111900324593204169285691081717;
    proof.opening_at_zeta_proof.X = 13224527132371105279913250536758217546265152888112369856898250310768235198131;
    proof.opening_at_zeta_proof.Y = 19302896778185915684708615973130767719760274252616248185517862385997672256778;
    proof.opening_at_zeta_omega_proof.X = 5327606518865781512867344072729252925384850742898184104533960334549231249970;
    proof.opening_at_zeta_omega_proof.Y = 4147756127288704823083405587555355700553844930947669522662884986348621037601;
    
    vk.domain_size = 32;
    vk.omega = 4419234939496763621076330863786513495701855246241724391626358375488475697872;
    vk.selector_commitments[0].X = 3249492299937356830250489011041180308067992016591401527068121784106989719648;
    vk.selector_commitments[0].Y = 10459965615643388455781136436726437288800547058370943251873623010731177440661;
    vk.selector_commitments[1].X = 12510476613922141136476828275709042037770171239066681610748147345655672163851;
    vk.selector_commitments[1].Y = 21702499139579688323831199788191067119894864133780232136805753631080002427269;
    vk.selector_commitments[2].X = 14953002130617700035755035451150408651119074291254331128989112575148233333491;
    vk.selector_commitments[2].Y = 17892566681051922084336151301309366102531970850688837636319063607093137053627;
    vk.selector_commitments[3].X = 12510476613922141136476828275709042037770171239066681610748147345655672163851;
    vk.selector_commitments[3].Y = 185743732259586898415205957066207968801447023517591525883284263565223781314;
    vk.selector_commitments[4].X = 14953002130617700035755035451150408651119074291254331128989112575148233333491;
    vk.selector_commitments[4].Y = 3995676190787353137910254443947908986164340306608986026369974287552089154956;
    vk.selector_commitments_commit_api = new Bn254.G1Point[](1);
    vk.selector_commitments_commit_api[0].X = 6072894980673347906024769411958097208049504128219463716820120075337948200814;
    vk.selector_commitments_commit_api[0].Y = 19560123544018144421880384701499189813392268921297788713816469086064585937291;
    vk.permutation_commitments[0].X = 21855018542748430565529761638971558125245342907512256948393636927196567938581;
    vk.permutation_commitments[0].Y = 11712367707713868753009749003773415568950091810241040629437353992390346924664;
    vk.permutation_commitments[1].X = 17771334109737095158037840313408192145908096951666120454569319380122548644876;
    vk.permutation_commitments[1].Y = 1557548382852739357942435662406820815086929855797636868167313245414326520716;
    vk.permutation_commitments[2].X = 3042622247313413937841956962385471739016337091363862127586520834001367730368;
    vk.permutation_commitments[2].Y = 11237012146990418046605498478831176936003562652049794077037238123223985118834;
    vk.coset_shift = 5;
    vk.g2_x.X0 = 4777846902900565418590449384753263717909657903692016614099552076160357595620;
    vk.g2_x.X1 = 3861286923073220011793349409046889289349533020715526625969101603056608090795;
    vk.g2_x.Y0 = 16406754891999554747479650379038048271643900448173543122927661446988296543616;
    vk.g2_x.Y1 = 21022748302362729781528857183979865986597752242747307653138221198529458362155;
    vk.commitment_indices = new uint256[](1);
    vk.commitment_indices[0] = 3;

    bool res = PlonkVerifier.verify(proof, vk, public_inputs);
    emit PrintBool(res);
    return res;
  }

  function test_plonk(uint256[] calldata kzg, bytes calldata preprocessed, bytes calldata proof, uint256[] calldata public_inputs) public returns (bool) {
    Types.Proof memory proofD = Marshal.deserialize_proof(proof);
    Types.VerificationKey memory vk = Marshal.deserialize_vk(kzg, preprocessed);
    bool res = PlonkVerifier.verify(proofD, vk, public_inputs);
    return true;
  }

  function test_plonk_deserialize(uint256[] calldata kzg, bytes calldata preprocessed, bytes calldata proof, uint256[] calldata public_inputs) public returns (bool) {
    Types.Proof memory proofD = Marshal.deserialize_proof(proof);
    Types.VerificationKey memory vk = Marshal.deserialize_vk(kzg, preprocessed);
    Types.PartialVerifierState memory state;   
    // step 1: derive gamma, beta, alpha, delta
    PlonkVerifier.derive_gamma_beta_alpha_zeta(state, proofD, vk, public_inputs);
    return false;
  }
}