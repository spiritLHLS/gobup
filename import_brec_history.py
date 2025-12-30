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
            
            # 提取所有字段
            room_id = root.find('.//RoomId').text if root.find('.//RoomId') is not None else ''
            short_id = root.find('.//ShortId').text if root.find('.//ShortId') is not None else ''
            name = root.find('.//Name').text if root.find('.//Name') is not None else ''
            title = root.find('.//Title').text if root.find('.//Title') is not None else ''
            area_parent = root.find('.//AreaNameParent').text if root.find('.//AreaNameParent') is not None else ''
            area_child = root.find('.//AreaNameChild').text if root.find('.//AreaNameChild') is not None else ''
            start_time = root.find('.//StartTime').text if root.find('.//StartTime') is not None else ''
            end_time = root.find('.//EndTime').text if root.find('.//EndTime') is not None else ''
            session_id = root.find('.//SessionId').text if root.find('.//SessionId') is not None else ''
            
            # 关键：如果 SessionId 为空，使用 StartTime 生成唯一标识
            # 同一场直播的多个文件会有相同的 StartTime，从而共享相同的 session_id
            if not session_id and start_time:
                # 使用 room_id + start_time 作为 session_id
                # 这样同一场直播的所有文件都会有相同的 session_id
                import hashlib
                session_key = f"{room_id}_{start_time}"
                session_id = hashlib.md5(session_key.encode()).hexdigest()[:16]
            
            metadata = {
                'room_id': room_id,
                'short_id': short_id,
                'name': name,
                'title': title,
                'area_name_parent': area_parent,
                'area_name_child': area_child,
                'start_time': start_time,
                'end_time': end_time,
                'session_id': session_id,
            }
            
            import os
            if os.getenv('DEBUG'):
                print(f"   🔍 XML解析: RoomID={room_id}, Title={title}, SessionID={session_id[:8]}...")
            
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
            # API 可能返回 {"list": [...]} 或直接返回数组 [...]
            if isinstance(data, dict):
                histories = data.get('list', [])
            else:
                histories = data if isinstance(data, list) else []
            
            # 遍历所有历史记录，检查是否有相同的文件路径
            for history in histories:
                history_id = history.get('id') if isinstance(history, dict) else None
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
                    # API 可能返回 {"list": [...]} 或直接返回数组 [...]
                    if isinstance(parts_data, dict):
                        parts = parts_data.get('list', [])
                    else:
                        parts = parts_data if isinstance(parts_data, list) else []
                    
                    for part in parts:
                        if isinstance(part, dict) and part.get('filePath') == container_path:
                            return True
            
            return False
            
        except Exception as e:
            print(f"⚠️  检查文件是否存在时出错: {e}")
            return False
    
    def check_room_exists(self, room_id: str) -> bool:
        """检查房间是否已在 gobup 中配置"""
        try:
            response = self.session.post(
                f'{self.gobup_url}/api/room',
                json={},
                timeout=10
            )
            
            if response.status_code != 200:
                return False
            
            data = response.json()
            if isinstance(data, dict):
                rooms = data.get('list', [])
            else:
                rooms = data if isinstance(data, list) else []
            
            for room in rooms:
                if isinstance(room, dict) and room.get('room_id') == room_id:
                    return True
            
            return False
        except:
            return False
    
    def create_webhook_event(self, video_file: Path, metadata: Dict) -> bool:
        """通过 webhook 接口创建历史记录"""
        try:
            # 转换为容器内路径
            container_path = str(video_file).replace(str(self.brec_dir), '/rec')
            
            # 安全地转换为整数（处理空字符串的情况）
            def safe_int(value, default=0):
                try:
                    return int(value) if value else default
                except (ValueError, TypeError):
                    return default
            
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
                    "RoomId": safe_int(metadata.get('room_id')),
                    "ShortId": safe_int(metadata.get('short_id')),
                    "Name": metadata.get('name', ''),
                    "Title": metadata.get('title', ''),
                    "AreaNameParent": metadata.get('area_name_parent', ''),
                    "AreaNameChild": metadata.get('area_name_child', ''),
                    "FileSize": self.get_file_size(video_file)
                }
            }
            
            # 添加调试信息
            import os
            if os.getenv('DEBUG'):
                import json
                print(f"   📤 发送数据: {json.dumps(event_data, indent=2, ensure_ascii=False)}")
            
            response = self.session.post(
                f'{self.gobup_url}/api/recordWebHook',
                json=event_data,
                timeout=30
            )
            
            if os.getenv('DEBUG'):
                print(f"   📥 响应状态: {response.status_code}")
                print(f"   📥 响应内容: {response.text}")
            
            if response.status_code == 200:
                # 给后台处理一点时间
                import time
                time.sleep(0.5)
                
                # 验证是否真的导入成功（检查数据库）
                if self.verify_import(container_path):
                    return True
                else:
                    print(f"   ⚠️  警告: API返回成功但数据库中未找到记录")
                    return False
            else:
                print(f"⚠️  导入失败 (HTTP {response.status_code}): {response.text}")
                return False
                
        except Exception as e:
            print(f"❌ 导入出错: {e}")
            import traceback
            if os.getenv('DEBUG'):
                traceback.print_exc()
            return False
    
    def verify_import(self, container_path: str) -> bool:
        """验证文件是否真的被导入到数据库"""
        try:
            import time
            # 多次重试，因为后台处理可能需要时间
            for i in range(3):
                if i > 0:
                    time.sleep(1)
                
                if self.check_part_exists_in_db(container_path):
                    return True
            return False
        except:
            return False
    
    def check_part_exists_in_db(self, container_path: str) -> bool:
        """检查文件是否在数据库中"""
        try:
            response = self.session.post(
                f'{self.gobup_url}/api/history/list',
                json={},
                timeout=10
            )
            
            if response.status_code != 200:
                return False
            
            data = response.json()
            if isinstance(data, dict):
                histories = data.get('list', [])
            else:
                histories = data if isinstance(data, list) else []
            
            for history in histories:
                history_id = history.get('id') if isinstance(history, dict) else None
                if not history_id:
                    continue
                
                parts_response = self.session.post(
                    f'{self.gobup_url}/api/part/list/{history_id}',
                    json={},
                    timeout=10
                )
                
                if parts_response.status_code == 200:
                    parts_data = parts_response.json()
                    if isinstance(parts_data, dict):
                        parts = parts_data.get('list', [])
                    else:
                        parts = parts_data if isinstance(parts_data, list) else []
                    
                    for part in parts:
                        if isinstance(part, dict) and part.get('filePath') == container_path:
                            return True
            
            return False
        except:
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
        
        # 直接从文件名提取信息（不再依赖XML，因为XML是弹幕文件）
        metadata = self.create_default_metadata(video_file)
        
        import os
        if os.getenv('DEBUG'):
            print(f"   📝 元数据: RoomID={metadata['room_id']}, Title={metadata['title']}, SessionID={metadata['session_id'][:8]}...")
        
        # 检查房间是否已添加到 gobup
        if not self.check_room_exists(metadata['room_id']):
            print(f"   ⚠️  房间 {metadata['room_id']} 未在 gobup 中配置，请先在 Web 界面添加此房间")
            self.stats['failed'] += 1
            self.stats['errors'].append(f"{video_file.name}: 房间未配置")
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
        
        # 从文件名中提取信息
        # 格式: 录制-5050-20251227-231202-161-古法精油高手.flv
        # 或: 5050-用户名/录制-5050-20251227-231202-161-标题.flv
        import re
        import hashlib
        
        # 尝试从文件名提取房间号
        room_id = '0'
        filename = video_file.stem  # 不含扩展名
        
        # 尝试多种模式
        patterns = [
            r'录制-(\d+)-',  # 录制-5050-...
            r'^(\d+)-',      # 5050-...
            r'[^\d](\d{4,})[^\d]',  # 任意位置的4位以上数字
        ]
        
        for pattern in patterns:
            match = re.search(pattern, filename)
            if match:
                room_id = match.group(1)
                break
        
        # 如果还是没找到，尝试从父目录名提取
        if room_id == '0':
            parent_name = video_file.parent.name
            match = re.search(r'(\d{4,})', parent_name)
            if match:
                room_id = match.group(1)
        
        # 从文件名中提取日期时间作为直播开始时间
        # 格式: 录制-5050-20251227-231202-161-古法精油高手.flv
        #              ^^^^^^^^ ^^^^^^
        #              日期      时间
        start_time = None
        datetime_match = re.search(r'(\d{8})-(\d{6})', filename)
        if datetime_match:
            date_str = datetime_match.group(1)  # 20251227
            time_str = datetime_match.group(2)  # 231202
            try:
                # 构造 ISO 时间格式
                start_time = f"{date_str[:4]}-{date_str[4:6]}-{date_str[6:8]}T{time_str[:2]}:{time_str[2:4]}:{time_str[4:6]}"
            except:
                pass
        
        if not start_time:
            start_time = mtime.isoformat()
        
        # 提取标题（文件名最后的中文部分）
        title_match = re.search(r'-([^-]+)$', filename)
        title = title_match.group(1) if title_match else filename
        
        # 关键：生成 session_id，使用 room_id + 日期时间（不含毫秒）
        # 这样同一场直播的多个文件会有相同的 session_id
        # 例如：录制-5050-20251227-231202-161-xxx.flv 和 录制-5050-20251227-231202-828-yyy.flv
        # 都会提取出 20251227-231202，从而得到相同的 session_id
        session_key = f"{room_id}_{start_time.split('.')[0]}"  # 移除毫秒部分
        session_id = hashlib.md5(session_key.encode()).hexdigest()[:16]
        
        import os
        if os.getenv('DEBUG'):
            print(f"   📝 从文件名提取: RoomID={room_id}, Title={title}, StartTime={start_time}, SessionID={session_id[:8]}...")
        
        return {
            'room_id': room_id,
            'short_id': '0',
            'name': f'房间{room_id}',
            'title': title,
            'area_name_parent': '',
            'area_name_child': '',
            'start_time': start_time,
            'end_time': mtime.isoformat(),
            'session_id': session_id,
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
