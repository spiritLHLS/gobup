#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
BililiveRecorder 历史记录导入工具
用于从 BililiveRecorder 录制文件夹批量导入历史记录到 gobup

使用方法:
    python3 import_brec_history.py --dir /root/bilirecord --url http://localhost:22380 --user root --pass passwd
"""

import os
import sys
import json
import argparse
import requests
from datetime import datetime
from pathlib import Path
import xml.etree.ElementTree as ET
from typing import Dict, List, Optional, Tuple

class BrecImporter:
    def __init__(self, brec_dir: str, gobup_url: str, username: str, password: str):
        self.brec_dir = Path(brec_dir)
        self.gobup_url = gobup_url.rstrip('/')
        self.auth = (username, password)
        self.session = requests.Session()
        self.session.auth = self.auth
        
        # 统计信息
        self.stats = {
            'total_files': 0,
            'success': 0,
            'skipped': 0,
            'failed': 0,
            'errors': []
        }
    
    def parse_xml_metadata(self, xml_path: Path) -> Optional[Dict]:
        """解析 BililiveRecorder 的 XML 元数据文件"""
        try:
            tree = ET.parse(xml_path)
            root = tree.getroot()
            
            metadata = {
                'room_id': root.find('.//RoomId').text if root.find('.//RoomId') is not None else '',
                'short_id': root.find('.//ShortId').text if root.find('.//ShortId') is not None else '',
                'name': root.find('.//Name').text if root.find('.//Name') is not None else '',
                'title': root.find('.//Title').text if root.find('.//Title') is not None else '',
                'area_name_parent': root.find('.//AreaNameParent').text if root.find('.//AreaNameParent') is not None else '',
                'area_name_child': root.find('.//AreaNameChild').text if root.find('.//AreaNameChild') is not None else '',
                'start_time': root.find('.//StartTime').text if root.find('.//StartTime') is not None else '',
                'end_time': root.find('.//EndTime').text if root.find('.//EndTime') is not None else '',
                'session_id': root.find('.//SessionId').text if root.find('.//SessionId') is not None else '',
            }
            
            return metadata
        except Exception as e:
            print(f"⚠️  解析 XML 失败 {xml_path}: {e}")
            return None
    
    def get_file_size(self, file_path: Path) -> int:
        """获取文件大小"""
        try:
            return file_path.stat().st_size
        except:
            return 0
    
    def parse_iso_time(self, time_str: str) -> Optional[str]:
        """解析 ISO 时间格式为 Go 能识别的格式"""
        if not time_str:
            return None
        try:
            # BililiveRecorder 使用 ISO 8601 格式，例如: 2023-12-30T10:30:00.000+08:00
            dt = datetime.fromisoformat(time_str.replace('Z', '+00:00'))
            # 返回 RFC3339 格式
            return dt.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + dt.strftime('%z')
        except Exception as e:
            print(f"⚠️  时间解析失败 {time_str}: {e}")
            return None
    
    def check_part_exists(self, file_path: str) -> bool:
        """检查分P是否已存在（通过文件路径去重）"""
        try:
            # 注意：容器内的路径需要转换
            # 宿主机: /root/bilirecord/xxx -> 容器内: /rec/xxx
            container_path = file_path.replace(str(self.brec_dir), '/rec')
            
            response = self.session.post(
                f'{self.gobup_url}/api/history/list',
                json={},
                timeout=10
            )
            
            if response.status_code != 200:
                return False
            
            data = response.json()
            histories = data.get('list', [])
            
            # 遍历所有历史记录，检查是否有相同的文件路径
            for history in histories:
                history_id = history.get('id')
                if not history_id:
                    continue
                
                # 获取分P列表
                parts_response = self.session.post(
                    f'{self.gobup_url}/api/part/list/{history_id}',
                    json={},
                    timeout=10
                )
                
                if parts_response.status_code == 200:
                    parts_data = parts_response.json()
                    parts = parts_data.get('list', [])
                    for part in parts:
                        if part.get('filePath') == container_path:
                            return True
            
            return False
            
        except Exception as e:
            print(f"⚠️  检查文件是否存在时出错: {e}")
            return False
    
    def create_webhook_event(self, video_file: Path, metadata: Dict) -> bool:
        """通过 webhook 接口创建历史记录"""
        try:
            # 转换为容器内路径
            container_path = str(video_file).replace(str(self.brec_dir), '/rec')
            
            # 构造 BililiveRecorder webhook 事件
            event_data = {
                "EventType": "FileClosed",
                "EventTimestamp": metadata.get('end_time', datetime.now().isoformat()),
                "EventId": metadata.get('session_id', ''),
                "EventData": {
                    "RelativePath": video_file.name,
                    "FileOpenTime": metadata.get('start_time', ''),
                    "FileCloseTime": metadata.get('end_time', ''),
                    "FilePath": container_path,
                    "SessionId": metadata.get('session_id', ''),
                    "RoomId": int(metadata.get('room_id', 0)),
                    "ShortId": int(metadata.get('short_id', 0)),
                    "Name": metadata.get('name', ''),
                    "Title": metadata.get('title', ''),
                    "AreaNameParent": metadata.get('area_name_parent', ''),
                    "AreaNameChild": metadata.get('area_name_child', ''),
                    "FileSize": self.get_file_size(video_file)
                }
            }
            
            response = self.session.post(
                f'{self.gobup_url}/api/recordWebHook',
                json=event_data,
                timeout=30
            )
            
            if response.status_code == 200:
                return True
            else:
                print(f"⚠️  导入失败 (HTTP {response.status_code}): {response.text}")
                return False
                
        except Exception as e:
            print(f"❌ 导入出错: {e}")
            return False
    
    def scan_and_import(self):
        """扫描目录并导入"""
        print(f"🔍 开始扫描目录: {self.brec_dir}")
        print(f"📡 gobup 地址: {self.gobup_url}")
        print("-" * 60)
        
        if not self.brec_dir.exists():
            print(f"❌ 目录不存在: {self.brec_dir}")
            return
        
        # 查找所有视频文件
        video_extensions = {'.flv', '.mp4', '.mkv'}
        video_files = []
        
        for ext in video_extensions:
            video_files.extend(self.brec_dir.rglob(f'*{ext}'))
        
        self.stats['total_files'] = len(video_files)
        print(f"📹 找到 {len(video_files)} 个视频文件\n")
        
        for video_file in sorted(video_files):
            self.process_video_file(video_file)
        
        self.print_summary()
    
    def process_video_file(self, video_file: Path):
        """处理单个视频文件"""
        print(f"📄 处理: {video_file.name}")
        
        # 查找对应的 XML 文件
        xml_file = video_file.with_suffix('.xml')
        
        if not xml_file.exists():
            print(f"   ⚠️  未找到元数据文件: {xml_file.name}")
            # 尝试使用文件信息作为默认值
            metadata = self.create_default_metadata(video_file)
        else:
            metadata = self.parse_xml_metadata(xml_file)
            if not metadata:
                self.stats['failed'] += 1
                self.stats['errors'].append(f"{video_file.name}: 解析 XML 失败")
                return
        
        # 检查是否已导入
        if self.check_part_exists(str(video_file)):
            print(f"   ⏭️  已存在，跳过")
            self.stats['skipped'] += 1
            return
        
        # 导入
        if self.create_webhook_event(video_file, metadata):
            print(f"   ✅ 导入成功")
            self.stats['success'] += 1
        else:
            print(f"   ❌ 导入失败")
            self.stats['failed'] += 1
            self.stats['errors'].append(f"{video_file.name}: 导入失败")
    
    def create_default_metadata(self, video_file: Path) -> Dict:
        """为没有 XML 的文件创建默认元数据"""
        stat = video_file.stat()
        mtime = datetime.fromtimestamp(stat.st_mtime)
        
        # 尝试从文件名中提取房间号（假设格式包含数字）
        room_id = '0'
        import re
        match = re.search(r'(\d{4,})', video_file.stem)
        if match:
            room_id = match.group(1)
        
        return {
            'room_id': room_id,
            'short_id': '0',
            'name': '未知主播',
            'title': video_file.stem,
            'area_name_parent': '',
            'area_name_child': '',
            'start_time': mtime.isoformat(),
            'end_time': mtime.isoformat(),
            'session_id': f"import_{video_file.stem}_{int(stat.st_mtime)}",
        }
    
    def print_summary(self):
        """打印统计摘要"""
        print("\n" + "=" * 60)
        print("📊 导入统计")
        print("=" * 60)
        print(f"总文件数: {self.stats['total_files']}")
        print(f"✅ 成功: {self.stats['success']}")
        print(f"⏭️  跳过: {self.stats['skipped']}")
        print(f"❌ 失败: {self.stats['failed']}")
        
        if self.stats['errors']:
            print("\n错误详情:")
            for error in self.stats['errors'][:10]:  # 只显示前10个错误
                print(f"  - {error}")
            if len(self.stats['errors']) > 10:
                print(f"  ... 还有 {len(self.stats['errors']) - 10} 个错误")


def main():
    parser = argparse.ArgumentParser(
        description='从 BililiveRecorder 录制文件夹批量导入历史记录到 gobup',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  # 基本用法
  python3 import_brec_history.py --dir /root/bilirecord

  # 指定 gobup 地址和认证信息
  python3 import_brec_history.py \\
    --dir /root/bilirecord \\
    --url http://localhost:22380 \\
    --user root \\
    --pass spiritlhl

  # 使用环境变量
  export GOBUP_URL=http://localhost:22380
  export GOBUP_USER=root
  export GOBUP_PASS=spiritlhl
  python3 import_brec_history.py --dir /root/bilirecord
        """
    )
    
    parser.add_argument(
        '--dir', '-d',
        required=True,
        help='BililiveRecorder 录制文件夹路径 (例如: /root/bilirecord)'
    )
    
    parser.add_argument(
        '--url', '-u',
        default=os.getenv('GOBUP_URL', 'http://localhost:22380'),
        help='gobup API 地址 (默认: http://localhost:22380)'
    )
    
    parser.add_argument(
        '--user',
        default=os.getenv('GOBUP_USER'),
        help='gobup 用户名（未提供则会提示输入）'
    )
    
    parser.add_argument(
        '--pass', '-p',
        dest='password',
        default=os.getenv('GOBUP_PASS'),
        help='gobup 密码（未提供则会提示输入）'
    )
    
    args = parser.parse_args()
    
    # 如果未提供用户名，则提示输入
    username = args.user
    if not username:
        username = input("请输入 gobup 用户名: ").strip()
        if not username:
            print("❌ 错误: 用户名不能为空")
            sys.exit(1)
    
    # 如果未提供密码，则提示输入（隐藏输入）
    password = args.password
    if not password:
        import getpass
        password = getpass.getpass("请输入 gobup 密码: ")
        if not password:
            print("❌ 错误: 密码不能为空")
            sys.exit(1)
    
    # 创建导入器并执行
    importer = BrecImporter(
        brec_dir=args.dir,
        gobup_url=args.url,
        username=username,
        password=password
    )
    
    try:
        importer.scan_and_import()
    except KeyboardInterrupt:
        print("\n\n⚠️  用户中断")
        importer.print_summary()
        sys.exit(1)
    except Exception as e:
        print(f"\n❌ 发生错误: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == '__main__':
    main()
