import XCTest
@testable import NaiLuShared

final class PolicyTests: XCTestCase {
    func testSubscriptionTrustBoundary() throws {
        for url in ["http://nailu.pro/a", "https://nailu.pro.evil.invalid/a", "https://user:secret@nailu.pro/a", "https://nailu.pro:444/a"] { XCTAssertThrowsError(try NaiLuOrigin.subscription(url)) }
        let url = try NaiLuOrigin.subscription("https://nailu.pro/api/v1/client/subscribe?token=fixture&flag=meta")
        let query = URLComponents(url: url, resolvingAgainstBaseURL: false)!.queryItems!
        XCTAssertEqual(query.filter { $0.name == "flag" }.map(\.value), ["v2ray"])
        XCTAssertEqual(query.first { $0.name == "token" }?.value, "fixture")
    }
    func testExpiredAndExhaustedPlansDoNotConnect() throws {
        let decoder = JSONDecoder()
        let s = try decoder.decode(Subscription.self, from: Data(#"{"transfer_enable":1024,"u":600,"d":600,"expired_at":null,"subscribe_url":"https://nailu.pro/s"}"#.utf8))
        XCTAssertEqual(s.remaining, 0); XCTAssertFalse(s.usable())
        let expired = try decoder.decode(Subscription.self, from: Data(#"{"transfer_enable":1024,"u":0,"d":0,"expired_at":1,"subscribe_url":"https://nailu.pro/s"}"#.utf8))
        XCTAssertFalse(expired.usable())
    }
    func testTrafficAlwaysShowsGB() { XCTAssertEqual(Traffic.gigabytes(4_398_046_511_104), "4096.00 GB") }
    func testUntrustedTrafficCountersCannotOverflow() throws {
        let raw = #"{"transfer_enable":9223372036854775807,"u":9223372036854775807,"d":9223372036854775807,"subscribe_url":"https://nailu.pro/s"}"#
        let plan = try JSONDecoder().decode(Subscription.self, from: Data(raw.utf8))
        XCTAssertEqual(plan.remaining, 0)
    }
}
