#!/usr/bin/env python3
"""Tests for the amen search window arithmetic. Needs no librosa or numpy."""

import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import detect_amen


class AmenSearchWindowTest(unittest.TestCase):
    def test_short_service_searches_inside_the_video_instead_of_refusing(self):
        # 2026-09-06: a 3884s video, the cross at 24:05 (1445s) plus 20 min = 4320s.
        window = detect_amen.amen_search_window(3884, 4320, 90 * 60, service_start_sec=1445)
        self.assertIsNotNone(window)
        start, duration = window
        self.assertGreaterEqual(start, 1445 + detect_amen.MIN_SERVICE_SEC)
        self.assertGreater(duration, 0)
        self.assertLessEqual(start + duration, 3884)

    def test_2026_07_26_video_also_searches(self):
        # 4548s video, 5040s offset (cross at 1.2 min short of 24:00 + 20 min).
        window = detect_amen.amen_search_window(4548, 5040, 90 * 60, service_start_sec=1800)
        self.assertIsNotNone(window)
        start, duration = window
        self.assertLessEqual(start + duration, 4548)

    def test_long_video_keeps_the_configured_offset(self):
        self.assertEqual(detect_amen.amen_search_window(7200, 4320, 90 * 60, service_start_sec=1445), (4320, 2880))

    def test_search_duration_caps_the_window(self):
        self.assertEqual(detect_amen.amen_search_window(20000, 1200, 600, service_start_sec=0), (1200, 600))

    def test_fallback_keeps_the_last_search_duration_of_a_long_short_video(self):
        # Video ends before the offset but is longer than the search duration.
        start, duration = detect_amen.amen_search_window(3884, 4320, 1800, service_start_sec=0)
        self.assertEqual((start, duration), (3884 - 1800, 1800))

    def test_a_sliver_left_after_the_offset_falls_back_too(self):
        # 80s past the offset would miss an amen that ended before it.
        start, duration = detect_amen.amen_search_window(4400, 4320, 90 * 60, service_start_sec=1445)
        self.assertLess(start, 4320)
        self.assertEqual(start + duration, 4400)

    def test_video_ending_before_the_service_can_have_ended_is_refused(self):
        self.assertIsNone(detect_amen.amen_search_window(1500, 4320, 90 * 60, service_start_sec=1445))

    def test_window_shorter_than_the_amen_is_refused(self):
        self.assertIsNone(detect_amen.amen_search_window(2100, 4320, 90 * 60, service_start_sec=1445, min_window_sec=60))


if __name__ == "__main__":
    unittest.main()
