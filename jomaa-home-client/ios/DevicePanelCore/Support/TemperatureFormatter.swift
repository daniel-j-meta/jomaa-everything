import Foundation

public enum TemperatureFormatter {
    public static func signedPercentage(for rawValue: Double) -> String {
        let bounded = min(max(rawValue, 0), 100)
        let signed = Int((bounded * 2 - 100).rounded())
        return signed > 0 ? "+\(signed)%" : "\(signed)%"
    }

    public static func accessibilityValue(for rawValue: Double) -> String {
        let bounded = min(max(rawValue, 0), 100)
        if bounded < 50 {
            return "\(Int(((50 - bounded) * 2).rounded())) percent warm"
        }
        if bounded > 50 {
            return "\(Int(((bounded - 50) * 2).rounded())) percent cool"
        }
        return "neutral"
    }
}
