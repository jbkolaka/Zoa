/// Small formatting helpers shared across screens.
library;

/// Formats a points value with thousands separators: `1240` → `1,240`.
///
/// Points are the number users care most about, and an unseparated five-digit
/// balance is genuinely hard to read at a glance.
String formatPoints(int points) {
  final digits = points.abs().toString();
  final buffer = StringBuffer();

  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) buffer.write(',');
    buffer.write(digits[i]);
  }

  return points < 0 ? '-$buffer' : buffer.toString();
}

/// Formats a date as `20 Aug 2026`.
///
/// Written out rather than using `intl`: one format, one locale for now, and it
/// avoids pulling in a package plus its locale data for a single line.
String formatDate(DateTime date) {
  const months = [
    'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
    'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
  ];
  return '${date.day} ${months[date.month - 1]} ${date.year}';
}
