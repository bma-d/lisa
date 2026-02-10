#!/usr/bin/env python3
"""Append a breadcrumb entry to today's daily log file."""

import argparse
import datetime
import os
import sys

BREADCRUMBS_DIR = os.path.dirname(os.path.abspath(__file__))


def main():
    parser = argparse.ArgumentParser(description="Log a breadcrumb entry")
    parser.add_argument("description", help="Short description of the change")
    parser.add_argument("details", nargs="?", default="", help="Optional details")
    parser.add_argument("--file", default="", help="Related file path")
    args = parser.parse_args()

    now = datetime.datetime.now()
    filename = now.strftime("%y%m%d") + ".md"
    filepath = os.path.join(BREADCRUMBS_DIR, filename)

    header_needed = not os.path.exists(filepath)

    with open(filepath, "a") as f:
        if header_needed:
            f.write(f"# Breadcrumbs {now.strftime('%Y-%m-%d')}\n\n")

        ts = now.strftime("%H:%M:%S")
        file_ref = f" (`{args.file}`)" if args.file else ""
        f.write(f"- **{ts}**{file_ref}: {args.description}\n")
        if args.details:
            f.write(f"  {args.details}\n")

    print(f"Logged to {filepath}", file=sys.stderr)


if __name__ == "__main__":
    main()
