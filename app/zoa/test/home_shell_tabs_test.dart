/// Tests for the role rules behind the bottom navigation.
///
/// These assert on [zoaTabsFor] rather than on a pumped [HomeShell], because
/// pumping the shell builds every screen behind the bar — each of which wants
/// its own registered controller and fires a request on first build. A widget
/// test would spend all its setup on the screens and none of it on the rule.
///
/// The rule is worth pinning because it is the only place in the client where a
/// role *removes* a capability rather than adding one, and because the natural
/// way to write it — reusing `isCollector` — is wrong in a way nothing else
/// would catch.
library;

import 'package:flutter_test/flutter_test.dart';
import 'package:zoa/api/api_models.dart';
import 'package:zoa/screens/home_shell.dart';

ZoaUser _user(String role) => ZoaUser(
      id: 1,
      phoneNumber: '+254712000002',
      name: 'Joseph Kariuki',
      role: role,
      pointsBalance: 0,
      createdAt: DateTime(2026, 8, 1),
    );

void main() {
  test('a collector has no Recycle tab and keeps the Queue', () {
    expect(
      zoaTabsFor(_user('collector')),
      [ZoaTab.home, ZoaTab.rewards, ZoaTab.profile, ZoaTab.queue],
      reason: 'a collector verifies other people\'s submissions and logs none '
          'of its own',
    );
  });

  test('a plain recycler keeps Recycle and gets no working tabs', () {
    expect(
      zoaTabsFor(_user('user')),
      [ZoaTab.home, ZoaTab.recycle, ZoaTab.rewards, ZoaTab.profile],
    );
  });

  test('partner staff keep Recycle — the exclusion is collector-only', () {
    expect(
      zoaTabsFor(_user('partner_staff')),
      [ZoaTab.home, ZoaTab.recycle, ZoaTab.rewards, ZoaTab.profile,
        ZoaTab.verify],
    );
  });

  // The regression this file exists for. Admin inherits collector everywhere
  // else — `isCollector` admits it, and so does RequireRole on the server — so
  // writing the exclusion in terms of that getter would silently take Recycle
  // away from the one account that has to walk the whole demo.
  test('an admin keeps Recycle alongside every working tab', () {
    final tabs = zoaTabsFor(_user('admin'));

    expect(tabs, [
      ZoaTab.home,
      ZoaTab.recycle,
      ZoaTab.rewards,
      ZoaTab.profile,
      ZoaTab.queue,
      ZoaTab.verify,
    ]);
    expect(tabs, contains(ZoaTab.recycle),
        reason: 'admin must not inherit the collector exclusion');
  });

  test('a signed-out shell falls back to the plain recycler set', () {
    expect(
      zoaTabsFor(null),
      [ZoaTab.home, ZoaTab.recycle, ZoaTab.rewards, ZoaTab.profile],
    );
  });

  test('Home is always first, so the index fallback always resolves', () {
    for (final role in ['user', 'collector', 'partner_staff', 'admin']) {
      expect(zoaTabsFor(_user(role)).first, ZoaTab.home, reason: role);
    }
  });
}
