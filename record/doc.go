// Package record decodes and encodes the hub's stored records: the
// device/activity record and, as they are ported, command, keymap, macro and
// inputs records.
//
// Decoding keeps the record's raw bytes, and encoding starts from them, so a
// record read from the hub and written back unchanged is byte-identical,
// including bytes this package does not understand yet. Only the fields a
// caller changes are rewritten.
package record
