package engine

import "reflect"

// copyValue 复制变量和对外数据中的可变值，保留原始 Go 类型。
// @param value 原始值
// @return 独立副本
func copyValue(value any) any {
	if value == nil {
		return nil
	}
	return copyReflectValue(reflect.ValueOf(value), make(map[copyReference]reflect.Value)).Interface()
}

// copyReference 标识已复制的容器，避免循环引用导致无限递归。
type copyReference struct {
	kind    reflect.Type // 容器类型
	pointer uintptr      // 原始地址
	length  int          // 切片长度
}

// copyReflectValue 递归复制容器、指针和结构体的公开字段。
// @param value 原始反射值
// @param seen 已复制容器
// @return 复制结果
func copyReflectValue(value reflect.Value, seen map[copyReference]reflect.Value) reflect.Value {
	// 容器先登记再递归，保留共享关系并支持环形对象的失败回滚。
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type()).Elem()
		result.Set(copyReflectValue(value.Elem(), seen))
		return result
	case reflect.Map, reflect.Slice, reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		key := copyReference{kind: value.Type(), pointer: value.Pointer()}
		if value.Kind() == reflect.Slice {
			key.length = value.Len()
		}
		if result, ok := seen[key]; ok {
			return result
		}
		var result reflect.Value
		switch value.Kind() {
		case reflect.Map:
			result = reflect.MakeMapWithSize(value.Type(), value.Len())
			seen[key] = result
			iter := value.MapRange()
			for iter.Next() {
				result.SetMapIndex(iter.Key(), copyReflectValue(iter.Value(), seen))
			}
		case reflect.Slice:
			result = reflect.MakeSlice(value.Type(), value.Len(), value.Len())
			seen[key] = result
			for i := 0; i < value.Len(); i++ {
				result.Index(i).Set(copyReflectValue(value.Index(i), seen))
			}
		case reflect.Pointer:
			result = reflect.New(value.Type().Elem())
			seen[key] = result
			result.Elem().Set(copyReflectValue(value.Elem(), seen))
		}
		return result
	case reflect.Array:
		result := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(copyReflectValue(value.Index(i), seen))
		}
		return result
	case reflect.Struct:
		result := reflect.New(value.Type()).Elem()
		result.Set(value)
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).IsExported() {
				result.Field(i).Set(copyReflectValue(value.Field(i), seen))
			}
		}
		return result
	default:
		return value
	}
}
