#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
BililiveRecorder 历史记录导入工具（直接操作数据库版本）
用于从 BililiveRecorder 录制文件夹批量导入历史记录到 gobup

使用方法:
    python3 import_brec_history_db.py --dir /root/bilirecord --db /root/data/gobup.db
"""

import os
import sys
import sqlite3
import argparse
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional
import re
import hashlib

class BrecImporterDB:
    def __init__(self, brec_dir: str, db_path: str):
        self.brec_dir = Path(brec_dir)
        self.db_path = db_path
        self.conn = None
        
        # 统计信息
        self.stats = {
            'total_files': 0,
            'success': 0,
            'skipped': 0,
            'failed': 0,
            'errors': []
        }
    
    def connect_db(self):
        """连接到数据库"""
        try:
            self.conn = sqlite3.connect(self.db_path)
            self.conn.row_factory = sqlite3.Row
            print(f"✅ 数据库连接成功: {self.db_path}")
            
            # 检测表结构
            self.detect_schema()
            return True
        except Exception as e:
            print(f"❌ 数据库连接失败: {e}")
            return False
    
    def detect_schema(self):
        """检测数据库表结构，判断是否有新字段"""
        try:
            cursor = self.conn.cursor()
            
            # 检查 record_histories 表的字段
            cursor.execute("PRAGMA table_info(record_histories)")
            history_columns = {row[1] for row in cursor.fetchall()}
            self.has_danmaku_fields = 'danmaku_sent' in history_columns
            
            # 检查 record_history_parts 表的字段
            cursor.execute("PRAGMA table_info(record_history_parts)")
            part_columns = {row[1] for row in cursor.fetchall()}
            self.has_cid_field = 'cid' in part_columns
            self.has_duration_field = 'duration' in part_columns
            
            if os.getenv('DEBUG'):
                print(f"   📋 数据库字段检测:")
                print(f"      - danmaku字段: {'✅' if self.has_danmaku_fields else '❌'}")
                print(f"      - cid字段: {'✅' if self.has_cid_field else '❌'}")
                print(f"      - duration字段: {'✅' if self.has_duration_field else '❌'}")
        except Exception as e:
            print(f"⚠️  检测表结构失败，使用兼容模式: {e}")
            self.has_danmaku_fields = False
            self.has_cid_field = False
            self.has_duration_field = False
    
    def close_db(self):
        """关闭数据库连接"""
        if self.conn:
            self.conn.close()
    
    def check_part_exists(self, file_path: str) -> bool:
        """检查分P是否已存在（通过文件路径去重）"""
        try:
            cursor = self.conn.cursor()
            cursor.execute(
                "SELECT id FROM record_history_parts WHERE file_path = ?",
                (file_path,)
            )
            result = cursor.fetchone()
            return result is not None
        except Exception as e:
            print(f"⚠️  检查文件是否存在时出错: {e}")
            return False
    
    def check_room_exists(self, room_id: str) -> bool:
        """检查房间是否已在 gobup 中配置"""
        try:
            cursor = self.conn.cursor()
            cursor.execute(
                "SELECT id FROM record_rooms WHERE room_id = ?",
                (room_id,)
            )
            result = cursor.fetchone()
            
            if os.getenv('DEBUG'):
                if result:
                    print(f"   ✅ 找到房间: {room_id}")
                else:
                    print(f"   ❌ 未找到房间 {room_id}")
            
            return result is not None
        except Exception as e:
            if os.getenv('DEBUG'):
                print(f"   ❌ 检查房间出错: {e}")
            return False
    
    def get_or_create_history(self, metadata: Dict) -> Optional[int]:
        """获取或创建历史记录"""
        try:
            cursor = self.conn.cursor()
            
            # 检查是否已存在相同 session_id 的历史记录
            cursor.execute(
                "SELECT id FROM record_histories WHERE session_id = ?",
                (metadata['session_id'],)
            )
            result = cursor.fetchone()
            
            if result:
                history_id = result[0]
                if os.getenv('DEBUG'):
                    print(f"   📝 使用现有历史记录 ID: {history_id}")
                return history_id
            
            # 创建新的历史记录
            now = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
            start_time = metadata.get('start_time', now)
            end_time = metadata.get('end_time', now)
            
            # 根据表结构动态构建SQL
            if self.has_danmaku_fields:
                cursor.execute("""
                    INSERT INTO record_histories (
                        created_at, updated_at,
                        room_id, session_id, uname, title, area_name,
                        start_time, end_time,
                        recording, streaming, upload, publish,
                        code, file_size,
                        danmaku_sent, danmaku_count, files_moved,
                        video_state, video_state_desc
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, (
                    now, now,
                    metadata['room_id'],
                    metadata['session_id'],
                    metadata.get('name', f"房间{metadata['room_id']}"),
                    metadata.get('title', ''),
                    metadata.get('area_name_parent', ''),
                    start_time,
                    end_time,
                    0,  # recording
                    0,  # streaming
                    1,  # upload
                    0,  # publish
                    -1, # code
                    0,  # file_size
                    0,  # danmaku_sent
                    0,  # danmaku_count
                    0,  # files_moved
                    -1, # video_state
                    ''  # video_state_desc
                ))
            else:
                # 旧版本数据库，不包含弹幕相关字段
                cursor.execute("""
                    INSERT INTO record_histories (
                        created_at, updated_at,
                        room_id, session_id, uname, title, area_name,
                        start_time, end_time,
                        recording, streaming, upload, publish,
                        code, file_size
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, (
                    now, now,
                    metadata['room_id'],
                    metadata['session_id'],
                    metadata.get('name', f"房间{metadata['room_id']}"),
                    metadata.get('title', ''),
                    metadata.get('area_name_parent', ''),
                    start_time,
                    end_time,
                    0,  # recording
                    0,  # streaming
                    1,  # upload
                    0,  # publish
                    -1, # code
                    0   # file_size
                ))
            
            history_id = cursor.lastrowid
            self.conn.commit()
            
            if os.getenv('DEBUG'):
                print(f"   ✅ 创建新历史记录 ID: {history_id}")
            
            return history_id
            
        except Exception as e:
            print(f"❌ 创建历史记录失败: {e}")
            import traceback
            if os.getenv('DEBUG'):
                traceback.print_exc()
            return None
    
    def create_part(self, history_id: int, video_file: Path, metadata: Dict) -> bool:
        """创建分P记录"""
        try:
            cursor = self.conn.cursor()
            
            # 转换为容器内路径（如果需要）
            container_path = str(video_file).replace(str(self.brec_dir), '/rec')
            
            now = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
            start_time = metadata.get('start_time', now)
            end_time = metadata.get('end_time', now)
            
            # 根据表结构动态构建SQL
            if self.has_cid_field and self.has_duration_field:
                # 新版本数据库，包含 duration 和 cid 字段
                cursor.execute("""
                    INSERT INTO record_history_parts (
                        created_at,
                        history_id, room_id, session_id,
                        title, live_title, area_name,
                        file_path, file_name, file_size, duration,
                        start_time, end_time,
                        recording, upload, uploading,
                        file_delete, file_moved, page, xcode_state, cid
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, (
                    now,
                    history_id,
                    metadata['room_id'],
                    metadata['session_id'],
                    metadata.get('title', ''),
                    metadata.get('title', ''),
                    metadata.get('area_name_parent', ''),
                    container_path,
                    video_file.name,
                    self.get_file_size(video_file),
                    0,  # duration
                    start_time,
                    end_time,
                    0,  # recording
                    0,  # upload
                    0,  # uploading
                    0,  # file_delete
                    0,  # file_moved
                    0,  # page
                    0,  # xcode_state
                    0   # cid
                ))
            else:
                # 旧版本数据库，不包含 duration 和 cid 字段
                cursor.execute("""
                    INSERT INTO record_history_parts (
                        created_at,
                        history_id, room_id, session_id,
                        title, live_title, area_name,
                        file_path, file_name, file_size,
                        start_time, end_time,
                        recording, upload, uploading,
                        file_delete, file_moved, page, xcode_state
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, (
                    now,
                    history_id,
                    metadata['room_id'],
                    metadata['session_id'],
                    metadata.get('title', ''),
                    metadata.get('title', ''),
                    metadata.get('area_name_parent', ''),
                    container_path,
                    video_file.name,
                    self.get_file_size(video_file),
                    start_time,
                    end_time,
                    0,  # recording
                    0,  # upload
                    0,  # uploading
                    0,  # file_delete
                    0,  # file_moved
                    0,  # page
                    0   # xcode_state
                ))
            
            self.conn.commit()
            
            if os.getenv('DEBUG'):
                print(f"   ✅ 创建分P记录成功")
            
            return True
            
        except Exception as e:
            print(f"❌ 创建分P记录失败: {e}")
            import traceback
            if os.getenv('DEBUG'):
                traceback.print_exc()
            self.conn.rollback()
            return False
    
    def get_file_size(self, file_path: Path) -> int:
        """获取文件大小"""
        try:
            return file_path.stat().st_size
        except:
            return 0
    
    def create_default_metadata(self, video_file: Path) -> Dict:
        """为视频文件创建默认元数据"""
        stat = video_file.stat()
        mtime = datetime.fromtimestamp(stat.st_mtime)
        
        # 从文件名中提取信息
        # 格式: 录制-5050-20251227-231202-161-古法精油高手.flv
        room_id = '0'
        filename = video_file.stem
        
        # 尝试多种模式提取房间号
        patterns = [
            r'录制-(\d+)-',
            r'^(\d+)-',
            r'[^\d](\d{4,})[^\d]',
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
        
        # 从文件名中提取日期时间
        start_time = None
        datetime_match = re.search(r'(\d{8})-(\d{6})', filename)
        if datetime_match:
            date_str = datetime_match.group(1)
            time_str = datetime_match.group(2)
            try:
                start_time = f"{date_str[:4]}-{date_str[4:6]}-{date_str[6:8]} {time_str[:2]}:{time_str[2:4]}:{time_str[4:6]}"
            except:
                pass
        
        if not start_time:
            start_time = mtime.strftime('%Y-%m-%d %H:%M:%S')
        
        # 提取标题
        title_match = re.search(r'-([^-]+)$', filename)
        title = title_match.group(1) if title_match else filename
        
        # 生成 session_id（同一场直播的多个文件使用相同的 session_id）
        # 策略：使用 房间号 + 日期 作为session标识
        # 这样同一天同一房间的所有录制都会归为同一场直播
        # 从文件名提取日期部分：录制-5050-20251227-231202-161 → 20251227
        date_match = re.search(r'(\d{8})', filename)
        if date_match:
            date_part = date_match.group(1)  # YYYYMMDD
            session_key = f"{room_id}_{date_part}"
        else:
            # 降级方案：使用文件修改时间的日期
            session_key = f"{room_id}_{start_time[:10]}"
        session_id = hashlib.md5(session_key.encode()).hexdigest()[:16]
        
        if os.getenv('DEBUG'):
            print(f"   📝 元数据: RoomID={room_id}, Title={title}, StartTime={start_time}, SessionID={session_id[:8]}...")
        
        return {
            'room_id': room_id,
            'short_id': '0',
            'name': f'房间{room_id}',
            'title': title,
            'area_name_parent': '',
            'area_name_child': '',
            'start_time': start_time,
            'end_time': mtime.strftime('%Y-%m-%d %H:%M:%S'),
            'session_id': session_id,
        }
    
    def scan_and_import(self):
        """扫描目录并导入"""
        print(f"🔍 开始扫描目录: {self.brec_dir}")
        print(f"💾 数据库路径: {self.db_path}")
        print("-" * 60)
        
        if not self.brec_dir.exists():
            print(f"❌ 目录不存在: {self.brec_dir}")
            return
        
        if not self.connect_db():
            return
        
        try:
            # 查找所有视频文件
            video_extensions = {'.flv', '.mp4', '.mkv'}
            video_files = []
            
            for ext in video_extensions:
                video_files.extend(self.brec_dir.rglob(f'*{ext}'))
            
            self.stats['total_files'] = len(video_files)
            print(f"📹 找到 {len(video_files)} 个视频文件\n")
            
            for video_file in sorted(video_files):
                self.process_video_file(video_file)
            
        finally:
            self.close_db()
        
        self.print_summary()
    
    def process_video_file(self, video_file: Path):
        """处理单个视频文件"""
        print(f"📄 处理: {video_file.name}")
        
        # 创建元数据
        metadata = self.create_default_metadata(video_file)
        
        # 检查房间是否存在
        if not self.check_room_exists(metadata['room_id']):
            print(f"   ⚠️  房间 {metadata['room_id']} 未在 gobup 中配置，跳过")
            self.stats['skipped'] += 1
            self.stats['errors'].append(f"{video_file.name}: 房间未配置")
            return
        
        # 检查是否已导入
        container_path = str(video_file).replace(str(self.brec_dir), '/rec')
        if self.check_part_exists(container_path):
            print(f"   ⏭️  已存在，跳过")
            self.stats['skipped'] += 1
            return
        
        # 获取或创建历史记录
        history_id = self.get_or_create_history(metadata)
        if not history_id:
            print(f"   ❌ 创建历史记录失败")
            self.stats['failed'] += 1
            self.stats['errors'].append(f"{video_file.name}: 创建历史记录失败")
            return
        
        # 创建分P记录
        if self.create_part(history_id, video_file, metadata):
            print(f"   ✅ 导入成功")
            self.stats['success'] += 1
        else:
            print(f"   ❌ 导入失败")
            self.stats['failed'] += 1
            self.stats['errors'].append(f"{video_file.name}: 创建分P失败")
    
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
            for error in self.stats['errors'][:10]:
                print(f"  - {error}")
            if len(self.stats['errors']) > 10:
                print(f"  ... 还有 {len(self.stats['errors']) - 10} 个错误")


def main():
    parser = argparse.ArgumentParser(
        description='从 BililiveRecorder 录制文件夹批量导入历史记录到 gobup（直接操作数据库）',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  # 基本用法
  python3 import_brec_history_db.py --dir /root/bilirecord --db /root/data/gobup.db

  # 启用调试模式
  DEBUG=1 python3 import_brec_history_db.py --dir /root/bilirecord --db /root/data/gobup.db
        """
    )
    
    parser.add_argument(
        '--dir', '-d',
        required=True,
        help='BililiveRecorder 录制文件夹路径 (例如: /root/bilirecord)'
    )
    
    parser.add_argument(
        '--db',
        default='/root/data/gobup.db',
        help='gobup 数据库文件路径 (默认: /root/data/gobup.db)'
    )
    
    args = parser.parse_args()
    
    # 检查数据库文件是否存在
    if not os.path.exists(args.db):
        print(f"❌ 错误: 数据库文件不存在: {args.db}")
        sys.exit(1)
    
    # 创建导入器并执行
    importer = BrecImporterDB(
        brec_dir=args.dir,
        db_path=args.db
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