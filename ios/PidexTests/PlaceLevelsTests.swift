import Testing
@testable import Pidex

struct PlaceNamesTests {
    @Test func citiesLoseTheirSuffixOnlyWhenChinese() {
        #expect(PlaceNames.city("大连市") == "大连")
        #expect(PlaceNames.city("沙市") == "沙市")
        #expect(PlaceNames.city("Dalian") == "Dalian")
        #expect(PlaceNames.city("") == "")
    }

    @Test func landmarksLoseTheCityTheyBeginWith() {
        var p = Pidex_V1_Place()
        p.region = "浙江省"
        p.city = "杭州市"
        p.district = "滨江区"
        #expect(PlaceNames.area("杭州市北京航空航天大学国际创新研究院", in: p) == "北京航空航天大学国际创新研究院")
        #expect(PlaceNames.area("浙江省杭州市滨江区江南大道", in: p) == "江南大道")
        #expect(PlaceNames.area("杭州西湖", in: p) == "西湖")
        // Nothing left: the name stays.
        #expect(PlaceNames.area("杭州市", in: p) == "杭州市")
        #expect(PlaceNames.area("Xinghai Square", in: p) == "Xinghai Square")
    }

    @Test func chipsAreShortAndClean() {
        #expect(PlaceNames.shown(count("area", "杭州市北京航空航天大学国际创新研究院", 1, in: "杭州市")) == "北京航空航天大学国际创新研…")
        #expect(PlaceNames.shown(count("city", "大连市", 1, in: "中国")) == "大连")
        #expect(PlaceNames.shown(count("country", "中国", 1)) == "中国")
    }
}

struct PlaceLevelsTests {
    // Japan and China, two cities in each, a landmark in each city.
    let trips = [
        count("country", "Japan", 5), count("country", "China", 3),
        count("city", "Tokyo", 4, in: "Japan"), count("city", "Kyoto", 1, in: "Japan"),
        count("city", "Dalian", 2, in: "China"), count("city", "Shenyang", 1, in: "China"),
        count("area", "Shibuya", 4, in: "Tokyo"), count("area", "Xinghai Square", 2, in: "Dalian"),
    ]

    @Test func severalCountriesAreGroupedByCountry() {
        let l = PlaceLevels(trips, picked: nil)
        #expect(l.shown.map(\.name) == ["Japan", "China"])
        #expect(l.inside == nil)
    }

    @Test func aPickedCountryShowsItsCities() {
        for picked in ["Japan", "Kyoto"] {
            let l = PlaceLevels(trips, picked: picked)
            #expect(l.inside?.name == "Japan")
            #expect(l.shown.map(\.name) == ["Tokyo", "Kyoto"])
        }
    }

    @Test func oneCountryIsGroupedByCity() {
        let china = trips.filter { $0.name == "China" || $0.parent == "China" || $0.parent == "Dalian" }
        #expect(PlaceLevels(china, picked: nil).shown.map(\.name) == ["Dalian", "Shenyang"])
        #expect(PlaceLevels.count(china) == 2)
    }

    @Test func oneCityIsGroupedByLandmark() {
        let city = [count("country", "China", 6), count("city", "Hangzhou", 6, in: "China"),
                    count("area", "West Lake", 4, in: "Hangzhou"), count("area", "Lingyin Temple", 2, in: "Hangzhou")]
        #expect(PlaceLevels(city, picked: nil).shown.map(\.name) == ["West Lake", "Lingyin Temple"])
        #expect(PlaceLevels.count(city) == 2)
        // No landmarks named: the one city.
        #expect(PlaceLevels(Array(city.prefix(2)), picked: nil).shown.map(\.name) == ["Hangzhou"])
    }
}

private func count(_ level: String, _ name: String, _ n: Int32, in parent: String = "") -> Pidex_V1_PlaceCount {
    var c = Pidex_V1_PlaceCount()
    c.level = level
    c.name = name
    c.count = n
    c.parent = parent
    return c
}
