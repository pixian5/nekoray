find_package(Protobuf CONFIG REQUIRED)

set(PROTO_FILES
        go/grpc_server/gen/libcore.proto
        )

add_library(myproto STATIC ${PROTO_FILES})
target_link_libraries(myproto
        PUBLIC
        protobuf::libprotobuf
        )
target_include_directories(myproto PUBLIC ${CMAKE_CURRENT_BINARY_DIR})
# Workaround: CMake 4.x does not propagate protobuf include dirs via target_link_libraries
get_target_property(_protobuf_inc protobuf::libprotobuf INTERFACE_INCLUDE_DIRECTORIES)
if(_protobuf_inc)
    target_include_directories(myproto PUBLIC ${_protobuf_inc})
endif()

protobuf_generate(TARGET myproto LANGUAGE cpp)
