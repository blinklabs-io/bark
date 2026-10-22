#!/usr/bin/env python3
"""Add explicit nil checks to protoc-gen-connect-go descriptor lookups."""

from pathlib import Path
import re
import subprocess


ROOT = Path(__file__).resolve().parents[1]
FILES = sorted((ROOT / "proto").rglob("*connect.go"))
SERVICE_LOOKUP = re.compile(
    r'(?P<var>\w+) := (?P<file>\w+\.File_[\w]+)\.Services\(\)'
    r'\.ByName\("(?P<service>\w+)"\)\.Methods\(\)'
)
METHOD_LOOKUP = re.compile(
    r'connect\.WithSchema\((?P<var>\w+)\.ByName\("(?P<method>\w+)"\)\)'
)


def transform(path: Path) -> bool:
    source = path.read_text()
    service_matches = list(SERVICE_LOOKUP.finditer(source))
    if not service_matches:
        return False

    service_names = {}
    for match in service_matches:
        variable = match.group("var")
        service = match.group("service")
        service_names[variable] = service
        replacement = (
            f'{variable} := mustConnectServiceMethods('
            f'{match.group("file")}.Services(), "{service}")'
        )
        source = source.replace(match.group(0), replacement)

    def replace_method(match: re.Match[str]) -> str:
        variable = match.group("var")
        service = service_names.get(variable)
        if service is None:
            raise SystemExit(f"no service descriptor mapping for {variable} in {path}")
        return (
            f'connect.WithSchema(mustConnectMethod({variable}, "{service}", '
            f'"{match.group("method")}"))'
        )

    source, method_count = METHOD_LOOKUP.subn(replace_method, source)
    if not method_count:
        raise SystemExit(f"service descriptor found without methods in {path}")

    source = source.replace(
        'strings "strings"\n',
        'strings "strings"\n\tprotoreflect '
        '"google.golang.org/protobuf/reflect/protoreflect"\n',
        1,
    )
    if 'protoreflect "google.golang.org/protobuf/reflect/protoreflect"' not in source:
        raise SystemExit(f"could not add protoreflect import in {path}")

    helpers = '''
func mustConnectServiceMethods(services protoreflect.ServiceDescriptors, serviceName string) protoreflect.MethodDescriptors {
\tservice := services.ByName(protoreflect.Name(serviceName))
\tif service == nil {
\t\tpanic("generated Connect service descriptor missing: " + serviceName)
\t}
\treturn service.Methods()
}

func mustConnectMethod(methods protoreflect.MethodDescriptors, serviceName, methodName string) protoreflect.MethodDescriptor {
\tmethod := methods.ByName(protoreflect.Name(methodName))
\tif method == nil {
\t\tpanic("generated Connect method descriptor missing: " + serviceName + "." + methodName)
\t}
\treturn method
}
'''
    marker = "const _ = connect.IsAtLeastVersion1_13_0\n"
    if marker not in source:
        raise SystemExit(f"Connect version marker missing in {path}")
    source = source.replace(marker, marker + helpers, 1)
    path.write_text(source)
    return True


def main() -> None:
    changed = [str(path.relative_to(ROOT)) for path in FILES if transform(path)]
    if changed:
        subprocess.run(["gofmt", "-w", *changed], cwd=ROOT, check=True)
        print("Added explicit Connect descriptor checks in:")
        print("\n".join(changed))
    elif not FILES or any(
        "mustConnectServiceMethods" not in path.read_text() for path in FILES
    ):
        raise SystemExit("no generated Connect descriptor lookups were transformed")


if __name__ == "__main__":
    main()
