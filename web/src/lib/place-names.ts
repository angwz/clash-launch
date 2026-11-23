// 地点名称按界面语言显示（反馈 #114）。
//
// 省、市来自 ip2region：中国的地址是中文，其他国家多为英文。国家名已由 Intl.DisplayNames 按界面语言生成，
// 省市没有现成的离线数据可用（运行时不调用第三方服务），这里收录常见的部分：
//   中国的省级行政区与主要城市、美国各州、日本都道府县，以及机房常见的世界城市。
// 收录到的按界面语言显示；没收录的保留数据库原文。运营商名不翻译。
type Names = readonly [zh: string, en: string, ja: string];

// 中国：键是去掉「省/市/自治区/特别行政区」等后缀的中文名
const CN: Record<string, Names> = {
  北京: ["北京市", "Beijing", "北京市"],
  天津: ["天津市", "Tianjin", "天津市"],
  上海: ["上海市", "Shanghai", "上海市"],
  重庆: ["重庆市", "Chongqing", "重慶市"],
  河北: ["河北省", "Hebei", "河北省"],
  山西: ["山西省", "Shanxi", "山西省"],
  辽宁: ["辽宁省", "Liaoning", "遼寧省"],
  吉林: ["吉林省", "Jilin", "吉林省"],
  黑龙江: ["黑龙江省", "Heilongjiang", "黒竜江省"],
  江苏: ["江苏省", "Jiangsu", "江蘇省"],
  浙江: ["浙江省", "Zhejiang", "浙江省"],
  安徽: ["安徽省", "Anhui", "安徽省"],
  福建: ["福建省", "Fujian", "福建省"],
  江西: ["江西省", "Jiangxi", "江西省"],
  山东: ["山东省", "Shandong", "山東省"],
  河南: ["河南省", "Henan", "河南省"],
  湖北: ["湖北省", "Hubei", "湖北省"],
  湖南: ["湖南省", "Hunan", "湖南省"],
  广东: ["广东省", "Guangdong", "広東省"],
  海南: ["海南省", "Hainan", "海南省"],
  四川: ["四川省", "Sichuan", "四川省"],
  贵州: ["贵州省", "Guizhou", "貴州省"],
  云南: ["云南省", "Yunnan", "雲南省"],
  陕西: ["陕西省", "Shaanxi", "陝西省"],
  甘肃: ["甘肃省", "Gansu", "甘粛省"],
  青海: ["青海省", "Qinghai", "青海省"],
  内蒙古: ["内蒙古自治区", "Inner Mongolia", "内モンゴル自治区"],
  广西: ["广西壮族自治区", "Guangxi", "広西チワン族自治区"],
  西藏: ["西藏自治区", "Tibet", "チベット自治区"],
  宁夏: ["宁夏回族自治区", "Ningxia", "寧夏回族自治区"],
  新疆: ["新疆维吾尔自治区", "Xinjiang", "新疆ウイグル自治区"],
  香港: ["香港", "Hong Kong", "香港"],
  澳门: ["澳门", "Macau", "マカオ"],
  台湾: ["台湾", "Taiwan", "台湾"],
  // 主要城市
  广州: ["广州市", "Guangzhou", "広州市"],
  深圳: ["深圳市", "Shenzhen", "深セン市"],
  东莞: ["东莞市", "Dongguan", "東莞市"],
  佛山: ["佛山市", "Foshan", "仏山市"],
  珠海: ["珠海市", "Zhuhai", "珠海市"],
  杭州: ["杭州市", "Hangzhou", "杭州市"],
  宁波: ["宁波市", "Ningbo", "寧波市"],
  温州: ["温州市", "Wenzhou", "温州市"],
  南京: ["南京市", "Nanjing", "南京市"],
  苏州: ["苏州市", "Suzhou", "蘇州市"],
  无锡: ["无锡市", "Wuxi", "無錫市"],
  常州: ["常州市", "Changzhou", "常州市"],
  徐州: ["徐州市", "Xuzhou", "徐州市"],
  成都: ["成都市", "Chengdu", "成都市"],
  武汉: ["武汉市", "Wuhan", "武漢市"],
  西安: ["西安市", "Xi'an", "西安市"],
  长沙: ["长沙市", "Changsha", "長沙市"],
  郑州: ["郑州市", "Zhengzhou", "鄭州市"],
  济南: ["济南市", "Jinan", "済南市"],
  青岛: ["青岛市", "Qingdao", "青島市"],
  烟台: ["烟台市", "Yantai", "煙台市"],
  沈阳: ["沈阳市", "Shenyang", "瀋陽市"],
  大连: ["大连市", "Dalian", "大連市"],
  哈尔滨: ["哈尔滨市", "Harbin", "ハルビン市"],
  长春: ["长春市", "Changchun", "長春市"],
  石家庄: ["石家庄市", "Shijiazhuang", "石家荘市"],
  太原: ["太原市", "Taiyuan", "太原市"],
  合肥: ["合肥市", "Hefei", "合肥市"],
  福州: ["福州市", "Fuzhou", "福州市"],
  厦门: ["厦门市", "Xiamen", "厦門市"],
  南昌: ["南昌市", "Nanchang", "南昌市"],
  昆明: ["昆明市", "Kunming", "昆明市"],
  贵阳: ["贵阳市", "Guiyang", "貴陽市"],
  南宁: ["南宁市", "Nanning", "南寧市"],
  海口: ["海口市", "Haikou", "海口市"],
  兰州: ["兰州市", "Lanzhou", "蘭州市"],
  西宁: ["西宁市", "Xining", "西寧市"],
  银川: ["银川市", "Yinchuan", "銀川市"],
  乌鲁木齐: ["乌鲁木齐市", "Ürümqi", "ウルムチ市"],
  拉萨: ["拉萨市", "Lhasa", "ラサ市"],
  呼和浩特: ["呼和浩特市", "Hohhot", "フフホト市"],
  台北: ["台北市", "Taipei", "台北市"],
  新北: ["新北市", "New Taipei", "新北市"],
  台中: ["台中市", "Taichung", "台中市"],
  高雄: ["高雄市", "Kaohsiung", "高雄市"],
};

// 美国各州（只在国家为 US 时使用：Georgia 等与国家同名）
const US: Record<string, Names> = {
  alabama: ["阿拉巴马州", "Alabama", "アラバマ州"],
  alaska: ["阿拉斯加州", "Alaska", "アラスカ州"],
  arizona: ["亚利桑那州", "Arizona", "アリゾナ州"],
  arkansas: ["阿肯色州", "Arkansas", "アーカンソー州"],
  california: ["加利福尼亚州", "California", "カリフォルニア州"],
  colorado: ["科罗拉多州", "Colorado", "コロラド州"],
  connecticut: ["康涅狄格州", "Connecticut", "コネチカット州"],
  delaware: ["特拉华州", "Delaware", "デラウェア州"],
  florida: ["佛罗里达州", "Florida", "フロリダ州"],
  georgia: ["佐治亚州", "Georgia", "ジョージア州"],
  hawaii: ["夏威夷州", "Hawaii", "ハワイ州"],
  idaho: ["爱达荷州", "Idaho", "アイダホ州"],
  illinois: ["伊利诺伊州", "Illinois", "イリノイ州"],
  indiana: ["印第安纳州", "Indiana", "インディアナ州"],
  iowa: ["艾奥瓦州", "Iowa", "アイオワ州"],
  kansas: ["堪萨斯州", "Kansas", "カンザス州"],
  kentucky: ["肯塔基州", "Kentucky", "ケンタッキー州"],
  louisiana: ["路易斯安那州", "Louisiana", "ルイジアナ州"],
  maine: ["缅因州", "Maine", "メイン州"],
  maryland: ["马里兰州", "Maryland", "メリーランド州"],
  massachusetts: ["马萨诸塞州", "Massachusetts", "マサチューセッツ州"],
  michigan: ["密歇根州", "Michigan", "ミシガン州"],
  minnesota: ["明尼苏达州", "Minnesota", "ミネソタ州"],
  mississippi: ["密西西比州", "Mississippi", "ミシシッピ州"],
  missouri: ["密苏里州", "Missouri", "ミズーリ州"],
  montana: ["蒙大拿州", "Montana", "モンタナ州"],
  nebraska: ["内布拉斯加州", "Nebraska", "ネブラスカ州"],
  nevada: ["内华达州", "Nevada", "ネバダ州"],
  "new hampshire": ["新罕布什尔州", "New Hampshire", "ニューハンプシャー州"],
  "new jersey": ["新泽西州", "New Jersey", "ニュージャージー州"],
  "new mexico": ["新墨西哥州", "New Mexico", "ニューメキシコ州"],
  "new york": ["纽约州", "New York", "ニューヨーク州"],
  "north carolina": ["北卡罗来纳州", "North Carolina", "ノースカロライナ州"],
  "north dakota": ["北达科他州", "North Dakota", "ノースダコタ州"],
  ohio: ["俄亥俄州", "Ohio", "オハイオ州"],
  oklahoma: ["俄克拉何马州", "Oklahoma", "オクラホマ州"],
  oregon: ["俄勒冈州", "Oregon", "オレゴン州"],
  pennsylvania: ["宾夕法尼亚州", "Pennsylvania", "ペンシルベニア州"],
  "rhode island": ["罗得岛州", "Rhode Island", "ロードアイランド州"],
  "south carolina": ["南卡罗来纳州", "South Carolina", "サウスカロライナ州"],
  "south dakota": ["南达科他州", "South Dakota", "サウスダコタ州"],
  tennessee: ["田纳西州", "Tennessee", "テネシー州"],
  texas: ["得克萨斯州", "Texas", "テキサス州"],
  utah: ["犹他州", "Utah", "ユタ州"],
  vermont: ["佛蒙特州", "Vermont", "バーモント州"],
  virginia: ["弗吉尼亚州", "Virginia", "バージニア州"],
  washington: ["华盛顿州", "Washington", "ワシントン州"],
  "west virginia": ["西弗吉尼亚州", "West Virginia", "ウェストバージニア州"],
  wisconsin: ["威斯康星州", "Wisconsin", "ウィスコンシン州"],
  wyoming: ["怀俄明州", "Wyoming", "ワイオミング州"],
  "district of columbia": ["哥伦比亚特区", "District of Columbia", "コロンビア特別区"],
};

// 日本都道府县（只在国家为 JP 时使用）
const JP: Record<string, Names> = {
  hokkaido: ["北海道", "Hokkaido", "北海道"],
  aomori: ["青森县", "Aomori", "青森県"],
  iwate: ["岩手县", "Iwate", "岩手県"],
  miyagi: ["宫城县", "Miyagi", "宮城県"],
  akita: ["秋田县", "Akita", "秋田県"],
  yamagata: ["山形县", "Yamagata", "山形県"],
  fukushima: ["福岛县", "Fukushima", "福島県"],
  ibaraki: ["茨城县", "Ibaraki", "茨城県"],
  tochigi: ["栃木县", "Tochigi", "栃木県"],
  gunma: ["群马县", "Gunma", "群馬県"],
  saitama: ["埼玉县", "Saitama", "埼玉県"],
  chiba: ["千叶县", "Chiba", "千葉県"],
  tokyo: ["东京都", "Tokyo", "東京都"],
  kanagawa: ["神奈川县", "Kanagawa", "神奈川県"],
  niigata: ["新潟县", "Niigata", "新潟県"],
  toyama: ["富山县", "Toyama", "富山県"],
  ishikawa: ["石川县", "Ishikawa", "石川県"],
  fukui: ["福井县", "Fukui", "福井県"],
  yamanashi: ["山梨县", "Yamanashi", "山梨県"],
  nagano: ["长野县", "Nagano", "長野県"],
  gifu: ["岐阜县", "Gifu", "岐阜県"],
  shizuoka: ["静冈县", "Shizuoka", "静岡県"],
  aichi: ["爱知县", "Aichi", "愛知県"],
  mie: ["三重县", "Mie", "三重県"],
  shiga: ["滋贺县", "Shiga", "滋賀県"],
  kyoto: ["京都府", "Kyoto", "京都府"],
  osaka: ["大阪府", "Osaka", "大阪府"],
  hyogo: ["兵库县", "Hyogo", "兵庫県"],
  nara: ["奈良县", "Nara", "奈良県"],
  wakayama: ["和歌山县", "Wakayama", "和歌山県"],
  tottori: ["鸟取县", "Tottori", "鳥取県"],
  shimane: ["岛根县", "Shimane", "島根県"],
  okayama: ["冈山县", "Okayama", "岡山県"],
  hiroshima: ["广岛县", "Hiroshima", "広島県"],
  yamaguchi: ["山口县", "Yamaguchi", "山口県"],
  tokushima: ["德岛县", "Tokushima", "徳島県"],
  kagawa: ["香川县", "Kagawa", "香川県"],
  ehime: ["爱媛县", "Ehime", "愛媛県"],
  kochi: ["高知县", "Kochi", "高知県"],
  fukuoka: ["福冈县", "Fukuoka", "福岡県"],
  saga: ["佐贺县", "Saga", "佐賀県"],
  nagasaki: ["长崎县", "Nagasaki", "長崎県"],
  kumamoto: ["熊本县", "Kumamoto", "熊本県"],
  oita: ["大分县", "Oita", "大分県"],
  miyazaki: ["宫崎县", "Miyazaki", "宮崎県"],
  kagoshima: ["鹿儿岛县", "Kagoshima", "鹿児島県"],
  okinawa: ["冲绳县", "Okinawa", "沖縄県"],
};

// 世界常见城市（机房与节点常见地）：不限国家
const CITIES: Record<string, Names> = {
  "los angeles": ["洛杉矶", "Los Angeles", "ロサンゼルス"],
  "san jose": ["圣何塞", "San Jose", "サンノゼ"],
  "san francisco": ["旧金山", "San Francisco", "サンフランシスコ"],
  "santa clara": ["圣克拉拉", "Santa Clara", "サンタクララ"],
  fremont: ["弗里蒙特", "Fremont", "フリーモント"],
  seattle: ["西雅图", "Seattle", "シアトル"],
  "new york city": ["纽约", "New York City", "ニューヨーク"],
  chicago: ["芝加哥", "Chicago", "シカゴ"],
  dallas: ["达拉斯", "Dallas", "ダラス"],
  miami: ["迈阿密", "Miami", "マイアミ"],
  atlanta: ["亚特兰大", "Atlanta", "アトランタ"],
  ashburn: ["阿什本", "Ashburn", "アッシュバーン"],
  phoenix: ["凤凰城", "Phoenix", "フェニックス"],
  denver: ["丹佛", "Denver", "デンバー"],
  "las vegas": ["拉斯维加斯", "Las Vegas", "ラスベガス"],
  yokohama: ["横滨", "Yokohama", "横浜"],
  singapore: ["新加坡", "Singapore", "シンガポール"],
  "hong kong": ["香港", "Hong Kong", "香港"],
  taipei: ["台北", "Taipei", "台北"],
  seoul: ["首尔", "Seoul", "ソウル"],
  london: ["伦敦", "London", "ロンドン"],
  frankfurt: ["法兰克福", "Frankfurt", "フランクフルト"],
  "frankfurt am main": ["法兰克福", "Frankfurt am Main", "フランクフルト"],
  amsterdam: ["阿姆斯特丹", "Amsterdam", "アムステルダム"],
  paris: ["巴黎", "Paris", "パリ"],
  berlin: ["柏林", "Berlin", "ベルリン"],
  sydney: ["悉尼", "Sydney", "シドニー"],
  melbourne: ["墨尔本", "Melbourne", "メルボルン"],
  toronto: ["多伦多", "Toronto", "トロント"],
  vancouver: ["温哥华", "Vancouver", "バンクーバー"],
  montreal: ["蒙特利尔", "Montreal", "モントリオール"],
  moscow: ["莫斯科", "Moscow", "モスクワ"],
  mumbai: ["孟买", "Mumbai", "ムンバイ"],
  bangkok: ["曼谷", "Bangkok", "バンコク"],
  "kuala lumpur": ["吉隆坡", "Kuala Lumpur", "クアラルンプール"],
  jakarta: ["雅加达", "Jakarta", "ジャカルタ"],
  manila: ["马尼拉", "Manila", "マニラ"],
  hanoi: ["河内", "Hanoi", "ハノイ"],
  "ho chi minh city": ["胡志明市", "Ho Chi Minh City", "ホーチミン市"],
  dubai: ["迪拜", "Dubai", "ドバイ"],
  istanbul: ["伊斯坦布尔", "Istanbul", "イスタンブール"],
  madrid: ["马德里", "Madrid", "マドリード"],
  stockholm: ["斯德哥尔摩", "Stockholm", "ストックホルム"],
  zurich: ["苏黎世", "Zurich", "チューリッヒ"],
  "sao paulo": ["圣保罗", "São Paulo", "サンパウロ"],
};

const SUFFIX = /(特别行政区|壮族自治区|回族自治区|维吾尔自治区|自治区|省|市)$/;
// 汉字区间与组合变音符号区间用码位构造（源码中不写 Unicode 转义字面量）
const CJK = new RegExp("[" + String.fromCharCode(0x4e00) + "-" + String.fromCharCode(0x9fff) + "]");
const MARKS = new RegExp("[" + String.fromCharCode(0x300) + "-" + String.fromCharCode(0x36f) + "]", "g");
// 去掉变音符号与「Prefecture」等后缀后小写，用于英文名的查找
const norm = (s: string) =>
  s
    .normalize("NFD")
    .replace(MARKS, "")
    .replace(/\s+(prefecture|state)$/i, "")
    .toLowerCase()
    .trim();

/** 界面语言对应的名称列：zh-*、ja-* 与其余（英文）。 */
export function placeLang(locale: string): 0 | 1 | 2 {
  return locale.startsWith("zh") ? 0 : locale.startsWith("ja") ? 2 : 1;
}

/** 按界面语言显示一个省或市；没收录时返回原文。 */
export function placeName(name: string, cc: string | undefined, locale: string): string {
  const col = placeLang(locale);
  let hit: Names | undefined;
  if (CJK.test(name)) {
    if (col === 0) return name; // 中文界面直接用原文
    hit = CN[name.replace(SUFFIX, "")] ?? CN[name];
  } else {
    const k = norm(name);
    hit = (cc === "US" ? US[k] : cc === "JP" ? JP[k] : undefined) ?? CITIES[k];
  }
  return hit ? hit[col] : name;
}
