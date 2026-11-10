#!/usr/bin/env python3
"""把镜像 tag 写入 VOS 发布表（飞书电子表格）。

表结构：第 1 行服务名，第 2 行镜像仓库，A 列日期（从第 4 行起），单元格放 tag。

用法：
  feishu_sheet.py --sheet AMD_with_cuda --component garm \
                  --image swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm \
                  --value amd_v0.2.1_20261110.1.e8478193 [--date 20261110] [--dry-run]

凭据来源（环境变量优先）：~/.feishu.json、FEISHU_CONFIG_FILE 指向的文件。
字段：feishu_app_id、feishu_app_secret、feishu_spreadsheet_token。
对应环境变量：FEISHU_APP_ID、FEISHU_APP_SECRET、FEISHU_SPREADSHEET_TOKEN。
"""

import argparse
import datetime
import json
import os
import sys
import urllib.error
import urllib.request

API = "https://open.feishu.cn/open-apis"


def die(message):
    print("feishu_sheet.py: %s" % message, file=sys.stderr)
    raise SystemExit(1)


def request(method, url, body=None, token=None):
    data = json.dumps(body).encode() if body is not None else None
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode())
    except urllib.error.HTTPError as exc:
        die("%s %s failed: HTTP %s" % (method, url, exc.code))
    except urllib.error.URLError as exc:
        die("%s %s failed: %s" % (method, url, exc.reason))


def load_config(path):
    if not os.path.isfile(path):
        return {}
    try:
        with open(path, "r", encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, ValueError) as exc:
        die("cannot read %s: %s" % (path, exc))


def credentials():
    path = os.environ.get("FEISHU_CONFIG_FILE") or os.path.expanduser("~/.feishu.json")
    config = load_config(path)
    app_id = os.environ.get("FEISHU_APP_ID") or str(config.get("feishu_app_id") or "")
    app_secret = os.environ.get("FEISHU_APP_SECRET") or str(config.get("feishu_app_secret") or "")
    spreadsheet = os.environ.get("FEISHU_SPREADSHEET_TOKEN") or str(
        config.get("feishu_spreadsheet_token") or ""
    )
    for name, value in (
        ("FEISHU_APP_ID", app_id),
        ("FEISHU_APP_SECRET", app_secret),
        ("FEISHU_SPREADSHEET_TOKEN", spreadsheet),
    ):
        if not value:
            die("%s is empty; check %s" % (name, path))
    return app_id, app_secret, spreadsheet


def tenant_token(app_id, app_secret):
    data = request(
        "POST",
        API + "/auth/v3/tenant_access_token/internal",
        {"app_id": app_id, "app_secret": app_secret},
    )
    if data.get("code") != 0:
        die("token request failed: %s" % data)
    return data["tenant_access_token"]


def sheet_id(token, spreadsheet, title):
    data = request("GET", API + "/sheets/v3/spreadsheets/%s/sheets/query" % spreadsheet, token=token)
    if data.get("code") != 0:
        die("query sheets failed: %s" % data)
    for sheet in data.get("data", {}).get("sheets", []):
        if sheet.get("title") == title:
            return sheet["sheet_id"]
    die("sheet not found: %s" % title)


def cell_text(value):
    if value is None:
        return ""
    if isinstance(value, str):
        return value.strip()
    if isinstance(value, dict):
        return str(value.get("text") or value.get("link") or "").strip()
    if isinstance(value, list):
        return "".join(cell_text(item) for item in value).strip()
    return str(value).strip()


def read_values(token, spreadsheet, cell_range):
    url = API + "/sheets/v2/spreadsheets/%s/values/%s?valueRenderOption=ToString" % (
        spreadsheet,
        cell_range,
    )
    data = request("GET", url, token=token)
    if data.get("code") != 0:
        die("read %s failed: %s" % (cell_range, data))
    return data.get("data", {}).get("valueRange", {}).get("values", [])


def column_letter(index):
    letters = ""
    while index > 0:
        index, remainder = divmod(index - 1, 26)
        letters = chr(ord("A") + remainder) + letters
    return letters


def component_column(token, spreadsheet, sheet, name):
    rows = read_values(token, spreadsheet, "%s!A1:ZZ2" % sheet)
    names = rows[0] if rows else []
    repos = rows[1] if len(rows) > 1 else []
    width = max(len(names), len(repos))
    for index in range(2, width + 1):
        current = cell_text(names[index - 1] if index <= len(names) else "")
        if current == name:
            return index, False
    for index in range(2, width + 2):
        current = cell_text(names[index - 1] if index <= len(names) else "")
        repo = cell_text(repos[index - 1] if index <= len(repos) else "")
        if not current and not repo:
            return index, True
    return width + 1, True


def date_row(token, spreadsheet, sheet, date):
    rows = read_values(token, spreadsheet, "%s!A4:A2000" % sheet)
    for offset, row in enumerate(rows):
        if row and str(row[0]).strip() == date:
            return offset + 4
    return None


def prepend_date(token, spreadsheet, sheet, date):
    body = {"valueRange": {"range": "%s!A4:A4" % sheet, "values": [[date]]}}
    url = API + "/sheets/v2/spreadsheets/%s/values_prepend" % spreadsheet
    data = request("POST", url, body, token=token)
    if data.get("code") != 0:
        die("insert date row failed: %s" % data)
    return 4


def write_cell(token, spreadsheet, sheet, cell, value):
    body = {"valueRange": {"range": "%s!%s:%s" % (sheet, cell, cell), "values": [[value]]}}
    url = API + "/sheets/v2/spreadsheets/%s/values" % spreadsheet
    data = request("PUT", url, body, token=token)
    if data.get("code") != 0:
        die("write %s!%s failed: %s" % (sheet, cell, data))


def main():
    parser = argparse.ArgumentParser(add_help=True, description="write an image tag into the VOS release sheet")
    parser.add_argument("--sheet", required=True)
    parser.add_argument("--component", required=True)
    parser.add_argument("--image", required=True)
    parser.add_argument("--value", required=True)
    parser.add_argument("--date", default=None)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    date = args.date or datetime.date.today().strftime("%Y%m%d")
    if len(date) != 8 or not date.isdigit():
        die("invalid --date: %s" % date)

    app_id, app_secret, spreadsheet = credentials()
    token = tenant_token(app_id, app_secret)
    sheet = sheet_id(token, spreadsheet, args.sheet)
    index, is_new = component_column(token, spreadsheet, sheet, args.component)
    letter = column_letter(index)
    row = date_row(token, spreadsheet, sheet, date)

    if args.dry_run:
        print(
            "feishu_sheet.py: dry run %s!%s%s = %s (column new: %s, date row new: %s)"
            % (args.sheet, letter, row or 4, args.value, is_new, row is None)
        )
        return

    if row is None:
        row = prepend_date(token, spreadsheet, sheet, date)
        print("feishu_sheet.py: inserted date row %s in %s" % (date, args.sheet))
    if is_new:
        write_cell(token, spreadsheet, sheet, "%s1" % letter, args.component)
        write_cell(token, spreadsheet, sheet, "%s2" % letter, args.image)
        print("feishu_sheet.py: created column %s in %s" % (letter, args.sheet))
    write_cell(token, spreadsheet, sheet, "%s%d" % (letter, row), args.value)
    print("feishu_sheet.py: %s!%s%d = %s" % (args.sheet, letter, row, args.value))


if __name__ == "__main__":
    main()
